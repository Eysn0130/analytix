#!/usr/bin/env node

import fs from "node:fs";
import {
  expectedAnalytixToolBudgetForTask as registryExpectedAnalytixToolBudgetForTask,
  expectedAnalytixToolsForTask as registryExpectedAnalytixToolsForTask,
  formatEvalCoverageSummary,
  isReportTaskIdFromCapabilityRegistry,
  passiveNonfundsTaskIdsFromCapabilityRegistry,
  reportTaskIdsFromCapabilityRegistry as registryReportTaskIdsFromCapabilityRegistry,
  validateEvalCoverageContract,
  withEvalCoverageContract
} from "./eval-coverage-contract.mjs";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:18731";
const DEFAULT_CASE_ID = "be6a1df3d3c4";
const DEFAULT_HOLDER_NAME = "合成主体甲";
const DEFAULT_DATE_START = "2025-01-01";
const OUTPUT_DOC_POLLUTION_PATTERNS = [
  /蓝皮书/u,
  /执行方案/u,
  /\/goal/u,
  /\bdoctor\b/iu,
  /\bscore\b/iu,
  /diagnostic label/iu,
  /golden answer/iu,
  /\boracle\b/iu,
  /\brubric\b/iu,
];
const OUTPUT_REPORT_GATE_POLLUTION_PATTERNS = [
  /report gate/iu,
  /报告门禁/u,
  /write_blocked/iu,
  /门禁模板/u,
];
const SOURCE_DELIVERY_FAILURE_PATTERNS = [
  /弱来源(?!.*(不得|不能|不可|不应|不能作为|核验意见))/iu,
  /未验证计算(?!.*(不得|不能|不可|不应|不能作为|核验意见|验证状态))/iu,
  /raw\/source rows(?!.*(不得|不能|不可|不应|不能作为|核验意见))/iu,
  /raw rows(?!.*(不得|不能|不可|不应|不能作为|核验意见))/iu,
  /缺失交付(?!.*(不得|不能|不可|不应|不能作为|核验意见))/iu,
  /(?:已完成|已经完成|已生成)(?:[^。；\n]{0,24})(?:报告|notebook|证据包|图表)(?:[^。；\n]{0,24})(?:但未|未实际|没有实际)/iu,
];
const MODEL_EVAL_ANTI_PATTERN_GATES = [
  "continued discovery after first frontdoor card"
];
const INACTIVE_REPORT_SCENARIO_PATTERN = /(?:工程(?:项目)?|项目款|利益输送|串通投标|围标|行贿|受贿)/u;
const ORDINARY_FULL_CASE_TASK_IDS = new Set([
  "full_case_analysis_tree",
  "replay_full_case_simple_prompt_runs_tree"
]);
const ORDINARY_FULL_CASE_FORCED_TYPOLOGY_PATTERN = /(?:团伙|共同控制|对公向个人|现金同存同取|正常成本|利益输送|资产消费|投资理财|支付通道|商户平台|虚拟资产|OTC|境外|跨境|票税合同|工程(?:项目)?|项目款|串通投标|围标|行贿|受贿)/iu;

export const MODES = [
  {
    id: "plugin_disabled",
    label: "无插件 Codex",
    instruction: "在 Analytix 涉案资金研判插件未启用条件下完成任务。允许你按原生 Codex 思路探索，但必须实际读取案件 DuckDB 或等价数据源后再给金额结论，并说明数据来源、口径风险和不确定性。"
  },
  {
    id: "skill_only",
    label: "仅短 skill",
    instruction: "只注入 Analytix 涉案资金研判的短工作流纪律，不启用语义事实能力；必须实际读取案件 DuckDB 或等价数据源后再给金额结论，用于区分 prompt/skill 纪律本身带来的提升。"
  },
  {
    id: "plugin_enabled_passive",
    label: "插件开启-自然触发",
    instruction: "插件已启用，但不强制指定工具入口；请按原生 Codex 的侦查意图理解自然选择是否调用 Analytix 涉案资金研判工具。若给出金额、链路、报告结论，必须完成确定性事实复核，不要输出内部编号。"
  },
  {
    id: "skill_mcp",
    label: "skill + evidence diagnostic",
    instruction: "插件已启用；优先选择最小充分的 Analytix 语义事实工具，只有问题模糊且工具不明确时才用 funds_investigate navigator。继续追一层、改统计范围、指定新对象或提出新假设时可直接选择新的语义工具。明确自定义统计且语义工具不足时进入当前案件、只读、清洗表/分析索引、限行、可回放的受控专项核算；金额、链路和交付状态只能来自当前案件已执行的语义事实工具或受控专项核算，不要把来源不足、未执行的计算、来源明细或内部编号写成生产事实，不要让工具替代侦查假设。"
  },
  {
    id: "full_plugin",
    label: "完整插件",
    instruction: "插件已启用；按短 skill + 语义事实工具箱 + casegraph + 受控专项核算 + 证据复核 + 报告结论复核的路线执行。普通问题用案件范围、排行、画像、追踪、假设、质量、验证中的最小充分工具；funds_investigate 只是模糊自然语言 navigator；明确自定义统计且语义工具不足时进入当前案件、只读、清洗表/分析索引、限行、可回放的受控专项核算。正式报告级结论才追加报告结论复核。"
  }
];

const GOLDEN_ANSWER_SET = loadGoldenAnswerSet();
const CAPABILITY_REGISTRY = withEvalCoverageContract(loadCapabilityRegistry(), loadEvalCoverageFixture());

const TASK_TYPE_BY_TASK_ID = {
  bare_analytix_entry_status: "entry_status",
  case_context_card: "case_context",
  top_account: "ranking",
  data_quality: "data_quality",
  holder_scope: "object_dossier",
  account_dossier_card: "object_dossier",
  subject_dossier_person: "object_dossier",
  liuwenliang_to_zhangjinzhi: "counterparty",
  zhangjinzhi_continuation: "trace_continuation",
  graph_visualization_supported_edges: "graph_visualization",
  visual_evidence_delivery_pack: "visual_evidence",
  case_notebook_controlled_query: "case_workbench",
  old_thread_patterns: "investigation_lab",
  outflow_continuation: "trace_continuation",
  full_case_analysis_tree: "full_case_analysis",
  report_builder_generation: "report",
  full_report_gate: "report",
  case_3c72_top_rankings: "ranking",
  case_3c72_report_claim_review: "claim_qa",
  case_e7e3_top_rankings: "ranking",
  case_e7e3_quality_boundary: "data_quality",
  investigation_lab_cash_asset_hypotheses: "investigation_lab",
  passive_nonfunds_import_guidance: "passive_nonintervention",
  passive_nonfunds_copy_edit: "passive_nonintervention",
  passive_nonfunds_excel_formula: "passive_nonintervention",
  passive_nonfunds_meeting_todos: "passive_nonintervention",
  passive_nonfunds_fund_concept: "passive_nonintervention",
  passive_nonfunds_account_masking: "passive_nonintervention",
  report_mermaid_legal_guard: "claim_qa",
  report_candidate_cash_boundary_guard: "claim_qa",
  replay_import_clean_coverage: "data_quality",
  replay_multisubject_trace: "investigation_lab",
  replay_rescope_recompute: "quick_fact",
  replay_negative_search: "investigation_lab",
  replay_continue_one_more_hop: "trace_continuation",
  replay_report_materialize: "report",
  replay_amount_challenge: "data_quality",
  replay_full_case_simple_prompt_runs_tree: "full_case_analysis",
  replay_source_trace_upstream: "trace_continuation",
  replay_economic_crime_typology_leads: "investigation_lab",
  replay_case_delivery_pack: "report",
};

const BASE_TASKS = [
  {
    id: "bare_analytix_entry_status",
    title: "裸 /analytix 入口状态",
    commandHint: "/analytix",
    prompt: "只输入 /analytix。请只返回当前案件状态、可用分析入口和下一步建议；不得跑全案分析、不得生成报告、不得输出工具说明书或内部协议。",
    expectedTools: ["get_current_case", "get_case_scope_map"],
    expectedMarkers: ["当前案件", "可用", "下一步", "不生成报告", "全案分析"],
    oldThreadValue: "旧单前门会把裸入口带进总控/报告门禁；focused index 必须只是入口和状态。"
  },
  {
    id: "case_context_card",
    title: "当前案件上下文 Case Context Card",
    commandHint: "/analytix scope",
    prompt: "给我当前案件上下文卡。必须说明 active case、清洗表/分析索引数据范围、开户/联系电话/住址/IP/MAC/交易环境/商户渠道/调单反馈/查控措施覆盖口径、casegraph/资金图谱可用性、数据质量边界和建议进入的 focused skill；不得跑全案分析或报告，不得把原始表作为普通研判事实来源。",
    expectedTools: ["get_current_case", "get_case_scope_map", "get_scope_coverage", "get_casegraph"],
    expectedMarkers: ["Case Context", "active", "清洗", "开户", "联系电话", "住址", "IP", "MAC", "交易环境", "覆盖", "casegraph", "数据质量", "focused skill", "不生成报告"],
    oldThreadValue: "旧线程经常先自由扫库；case-context 必须给 Codex 案件上下文而不是替它做研判。"
  },
  {
    id: "top_account",
    title: "全案最大账户",
    commandHint: "/analytix rank",
    prompt: "该案中账户资金最大的账户是哪个？必须说明排序指标、全案规范交易覆盖数、账户维度覆盖数、数据质量边界和能否报告级使用。",
    expectedTools: ["rank_accounts"],
    expectedMarkers: ["133011759553CNY0", "285243", "540", "规范明细", "source", "同一户名"],
    oldThreadValue: "旧线程能直接查出 Top 账户，但容易出现行数/口径漂移；插件应给出同一 Top 并明确覆盖口径。"
  },
  {
    id: "data_quality",
    title: "导入清洗和重复风险",
    commandHint: "/analytix audit",
    prompt: "复核该案导入清洗是否可能存在同事实重复、换卡/补卡重复统计、户名空字段误读。尤其说明 clean_duplicate=0 是否等于没有重复。",
    expectedTools: ["audit_case_data_quality", "resolve_duplicate_families"],
    expectedMarkers: ["clean_duplicate=0", "30878", "158", "1", "同事实", "换卡", "不能全流水去重"],
    oldThreadValue: "旧线程能发现局部异常，但插件应把清洗风险、账户维表未登记和交易明细空户名分开。"
  },
  {
    id: "holder_scope",
    title: "合成主体甲账户集合",
    commandHint: "/analytix holder 合成主体甲",
    prompt: "合成主体甲登记账户集合交易量是多少？哪些属于登记账户，哪些只是待核账户线索？不要把待核账户线索写成已确认归属。",
    expectedTools: ["analyze_holder_full"],
    expectedMarkers: ["合成主体甲", "249", "登记账户", "待核账户线索", "5524622998.78"],
    oldThreadValue: "旧线程强在能围绕人名深挖；插件必须更强地区分直接账户、候选账户和未登记账户。"
  },
  {
    id: "account_dossier_card",
    title: "某卡资金研判账户资金画像",
    commandHint: "/analytix account <account>",
    prompt: "对账户 133011759553CNY0 做完整资金研判。请按公安经侦账户资金画像输出：账户基本情况、登记/开户信息、资金流入、资金流出、重点对手方、账户角色、异常特征、资金来源或去向线索、核验意见和核查建议；不能只回答一个 Top 或金额，不能把联系电话/住址/IP/MAC相同写成实际控制或同一操作人。",
    expectedTools: ["analyze_account_full", "rank_counterparties", "hypothesis_probe"],
    expectedMarkers: ["账户基本情况", "开户", "联系电话", "住址", "IP", "MAC", "交易环境", "资金流入", "资金流出", "重点对手方", "账户角色", "异常特征", "核验意见", "核查建议"],
    oldThreadValue: "旧线程能沿单卡做画像但成本高；focused skill 必须把某卡研判产品化为完整对象画像。"
  },
  {
    id: "subject_dossier_person",
    title: "某人资金研判 Object Dossier",
    commandHint: "/analytix holder <name>",
    prompt: "对合成主体甲做完整某人资金研判。请按公安经侦主体资金画像口吻输出：账户基本情况、登记账户清单、重点账户表、资金流入、资金流出、重点对手方、异常特征、可疑用途或去向、核验意见和核查建议；身份/开户事实、联系电话/住址/单位/法人/IP/MAC/交易环境/商户渠道等只作为关联线索或证据缺口说明。不得把待核账户线索或联系电话/住址/IP/MAC相同写成已确认归属、实际控制或团伙事实。",
    expectedTools: ["analyze_holder_full", "rank_accounts", "rank_counterparties"],
    expectedMarkers: ["账户基本情况", "登记账户清单", "重点账户表", "资金流入", "资金流出", "重点对手方", "异常特征", "核验意见", "核查建议", "开户", "联系电话", "住址", "IP", "MAC"],
    oldThreadValue: "旧线程能围绕某人多轮深挖；focused skill 必须一次形成完整主体画像。"
  },
  {
    id: "liuwenliang_to_zhangjinzhi",
    title: "合成主体甲转给合成主体乙金额核验",
    commandHint: "/analytix ask",
    prompt: "合成主体甲转给合成主体乙多少钱？请按经侦材料式回答：结论句、统计起止日期、有效去重金额和笔数、原明细金额、已确认重复金额、重点交易表、主要集中日/账户、异常特征、资金意义和下一步补证。不要使用旧工具卡标题，不要把已确认重复金额写成需复核是否重复，也不要把 4200 万写成事实。",
    expectedTools: ["rank_counterparties", "run_case_sql"],
    expectedMarkers: ["经梳理", "统计", "笔", "25,885,013", "21,000,000", "2025-08-27", "重点交易", "异常特征", "资金意义", "已确认重复", "不作为新增转账", "补调", "4200 万", "不能"],
    oldThreadValue: "旧线程靠自由 SQL 发现 42M 是重复放大。插件必须把这个发现转成两方金额核验 owner 的专业交付。"
  },
  {
    id: "zhangjinzhi_continuation",
    title: "合成主体乙后续去向",
    commandHint: "/analytix flowgraph",
    prompt: "合成主体甲转给合成主体乙后，合成主体乙又把钱转给谁？请给出确定性资金边、时间、金额、对手、不能确认的边界；不要手画无证据箭头。",
    expectedTools: ["build_fund_flow_graph", "trace_fund_next_hop"],
    expectedMarkers: ["合成主体乙", "浙银理财认申购户", "20000000", "20,000,000", "2025-08-27", "确定性资金边", "不得"],
    oldThreadValue: "旧线程能追到合成主体乙同日开户、再转 2000 万理财和少量现金/业务。插件必须更低 token 地给出同样或更清晰的边界。"
  },
  {
    id: "graph_visualization_supported_edges",
    title: "图谱可视化 supported edge 边界",
    commandHint: "/analytix flowgraph",
    prompt: "为当前案件做资金图谱/关系图可视化说明。请按经侦读图口吻写清结论、资金来源、主要资金链路、下游去向、资金断点和待补证事项；图中只画可证实交易边，旁路线索、缺端点、同名待核或聚合对象放入待核列表，不能画成确定箭头。",
    expectedTools: ["build_fund_flow_graph", "get_casegraph"],
    expectedMarkers: ["资金来源", "主要资金链路", "下游去向", "资金断点", "待补证事项", "资金流向图"],
    oldThreadValue: "旧线程能手动画图但容易把候选边画实；graph-visualization 必须把图谱作为事实上下文而非证据替代。"
  },
  {
    id: "visual_evidence_delivery_pack",
    title: "Visual Evidence 表格图表附件工作包",
    commandHint: "/analytix visual",
    prompt: "把当前全案分析结果整理成证据表格、Top20 图表、特征表、资金流向表、续调表和附件目录。必须说明每个表/图的统计范围、单位、时间窗口、指标、方向和支持程度；不得把排行、矩阵、看板或图表形状写成交易路径、控制关系或法律结论。",
    expectedTools: ["get_case_scope_map", "rank_accounts", "rank_holders", "rank_counterparties", "get_evidence_pack"],
    expectedMarkers: ["证据表格", "Top20", "特征表", "资金流向表", "续调表", "附件目录", "统计范围", "单位", "时间窗口", "指标", "方向", "支持程度"],
    oldThreadValue: "旧线程最终往往需要手工整理表格和材料；visual-evidence 必须把表格、图表、看板和附件工作包产品化，同时保持核验意见。"
  },
  {
    id: "case_notebook_controlled_query",
    title: "受控专项核算自定义统计",
    commandHint: "/analytix notebook",
    prompt: "现有语义工具无法覆盖一个自定义统计：按清洗明细和分析索引复核夜间大额出账占比，并要求生成可回放分析记录。应进入受控专项核算，说明当前案件、只读、清洗表/分析索引范围、允许视图、行数限制、执行审计要求、验证状态、核验意见、如果受控工具不可用时的能力缺口和下一步产品化沉淀；不得把来源不足、来源明细或未执行的计算写成生产事实。",
    expectedTools: ["get_case_scope_map", "inspect_case_schema", "audit_case_data_quality", "create_case_notebook", "run_case_sql"],
    expectedMarkers: ["受控专项核算", "当前案件", "只读", "清洗表", "分析索引", "限行", "可回放", "能力缺口", "验证状态", "核验意见"],
    oldThreadValue: "旧线程可用 SQL 自由探索自定义口径；插件必须把这种自由产品化为受控、限行、可回放的 workbench，而不是把工具边界误执行成能力封锁。"
  },
  {
    id: "old_thread_patterns",
    title: "旧线程式不规律性深挖",
    commandHint: "/analytix lab",
    prompt: "按历史行为基线围绕合成主体甲做开放式深挖，重点复核合成主体乙、理财、缺失对手大额、重复交易号、金额集中。每个结论要来自核验材料、交易边或确定性统计，不要输出内部引用编号、审计编号或附件编号。",
    expectedTools: ["hypothesis_probe"],
    expectedMarkers: ["7958757.83", "795.875783", "合成主体乙", "20250103175349102219900882174578", "financial_product", "候选差额"],
    oldThreadValue: "这是核心对比项。插件应保留自由假设，还要把旧线程细节变成可复跑核验材料。"
  },
  {
    id: "outflow_continuation",
    title: "Top 出账和补调清单",
    commandHint: "/analytix outflows",
    prompt: "对合成主体甲 2025 年以来 Top 出账进行终点分类和补调对象研判。不得把未匹配终点写成最终流向。",
    expectedTools: ["trace_subject_top_outflows"],
    expectedMarkers: ["Top", "补调", "终点", "合成主体乙", "续查清单"],
    oldThreadValue: "旧线程能顺着一笔钱往后查；插件应把 Top 种子、终点分类、无下游和附件 QA 结构化。"
  },
  {
    id: "full_case_analysis_tree",
    title: "全案分析树",
    commandHint: "/analytix plan",
    prompt: "对当前案件做全案分析，跑完整资金研判树，但不要生成正式报告。覆盖已核数据量、时间跨度、清洗表/分析索引覆盖、开户及交易环境材料覆盖、全案进出账金额和笔数、按人/账户统计、重点主体账户、账户角色、Top20、已核异常特征、可证实资金链路、证据缺口和续调清单。场景专题只能在同案宿主证据回执存在且来源能力支持具体主张时纳入；未激活专题整段省略，普通全案不得固定枚举所有案类专题。",
    expectedTools: ["run_full_case_analysis"],
    expectedMarkers: ["全案分析", "数据量", "时间跨度", "清洗", "开户", "交易环境", "账户角色", "进账", "出账", "Top20", "异常特征", "资金链路", "证据缺口", "续调"],
    oldThreadValue: "旧线程需要多轮才跑完全案；插件必须通过 full-case-analysis 形成完整全案分析树，且不误入报告生成。"
  },
  {
    id: "report_builder_generation",
    title: "明确报告生成",
    commandHint: "/analytix report",
    prompt: "明确生成当前案件全案资金研判报告草稿。必须先汇总已验证事实，再进入报告写作和报告结论复核；如果报告结论、来源边界、数据质量或未取得确定性资金边未通过，只能输出待复核报告边界和补证动作。",
    expectedTools: ["run_full_case_analysis", "validate_report_claims"],
    expectedMarkers: ["报告草稿", "已验证", "报告结论复核", "来源边界", "数据质量", "未取得确定性资金边", "需复核", "补证"],
    oldThreadValue: "旧线程能生成报告但口径风险高；report-builder 必须只在明确报告意图下材料化并复核。"
  },
  {
    id: "full_report_gate",
    title: "全案报告门禁",
    commandHint: "/analytix report",
    prompt: "生成全案资金研判报告草稿。如果来源核验、数据质量、报告结论复核或任何已激活专题的逐项证据复核未通过，必须暂缓出具最终报告；未激活专题整段省略，不得为满足模板补写。",
    expectedTools: ["run_full_case_analysis", "validate_report_claims"],
    expectedMarkers: ["报告结论复核", "来源核验", "数据质量", "暂缓出具", "需复核"],
    oldThreadValue: "旧线程报告内容丰富但容易口径漂移；插件必须做到报告前强制质检和阻断。"
  }
];

const REPLAY_TASKS = [
  {
    id: "replay_import_clean_coverage",
    title: "旧线程 replay: 导入清洗覆盖",
    commandHint: "/analytix audit",
    prompt: "复盘导入清洗覆盖、同事实重复、换卡/补卡、空户名与对手字段缺口；说明 clean_duplicate=0 的边界，不能把候选重复直接扣成事实。",
    expectedTools: ["audit_case_data_quality", "resolve_duplicate_families"],
    expectedMarkers: ["导入", "清洗", "clean_duplicate=0", "同事实", "换卡", "不能全流水去重"],
    oldThreadValue: "旧线程能做导入/清洗覆盖复核；插件应以核验材料给出相同能力并更清楚地区分数据事实和候选风险。"
  },
  {
    id: "replay_multisubject_trace",
    title: "旧线程 replay: 多主体资金追踪",
    commandHint: "/analytix lab",
    prompt: "围绕题目主体、关联公司和关联人做多主体资金追踪，输出已证实事实、可疑通道、核验意见、补调对象和下一步追查，不得误入正式报告门禁。",
    expectedTools: ["hypothesis_probe", "trace_subject_top_outflows"],
    expectedMarkers: ["开放式深挖", "多主体", "关联", "补调", "核验意见", "下一步"],
    oldThreadValue: "旧线程强在多主体追踪；插件必须保持开放假设和事实边界。"
  },
  {
    id: "replay_rescope_recompute",
    title: "旧线程 replay: 调整统计范围后重算",
    commandHint: "/analytix ask",
    prompt: "用户调整统计范围后重新计算金额、笔数和时间窗，明确完整数据范围、指定窗口、核心账户/交易号范围差异，不复用上一轮结论。",
    expectedTools: ["rank_counterparties"],
    expectedMarkers: ["统计范围", "重算", "实际期间", "时间窗口", "核心", "不能"],
    oldThreadValue: "旧线程能按用户调整范围重新跑数；插件不能被首卡收口边界压制。"
  },
  {
    id: "replay_negative_search",
    title: "旧线程 replay: 否定性搜索",
    commandHint: "/analytix ask",
    prompt: "复核某姓名/单位是否出现在清洗后的开户户名、对手户名、证件号、摘要备注、交易类型、IP/MAC/交易环境、商户渠道、人员表、联系电话表、住址表、调单反馈、查控措施、casegraph aliases；若未命中，要列明已检索清洗字段和核验意见，不能写成绝对不存在，也不能声称直接查了原始表。",
    expectedTools: ["hypothesis_probe"],
    expectedMarkers: ["未命中", "户名", "对手", "摘要", "备注", "IP", "MAC", "交易环境", "联系电话", "住址", "调单", "查控", "清洗字段", "边界"],
    oldThreadValue: "旧线程能做否定性搜索并说明字段范围；插件需同样给出边界而不是事实查询器式一句话。"
  },
  {
    id: "replay_continue_one_more_hop",
    title: "旧线程 replay: 继续追一层",
    commandHint: "/analytix flowgraph",
    prompt: "在已发现一跳收款对象后继续追一层，区分确定性交易边、后续对手统计、资产转换线索和逐笔余额承接缺口。",
    expectedTools: ["trace_fund_next_hop", "build_fund_flow_graph"],
    expectedMarkers: ["后续", "下一跳", "逐笔余额承接", "确定性资金边", "不能确认", "补调"],
    oldThreadValue: "旧线程可顺着一跳继续追；插件不能用 max_additional_tools=0 让后续深挖停住。"
  },
  {
    id: "replay_report_materialize",
    title: "旧线程 replay: 报告材料化",
    commandHint: "/analytix report",
    prompt: "把已复核事实材料化为报告草稿前，逐项检查报告结论、来源边界、数据质量、金额统计范围和未取得确定性资金边；不通过时只输出待复核草稿边界。",
    expectedTools: ["run_full_case_analysis", "validate_report_claims"],
    expectedMarkers: ["报告结论", "来源边界", "数据质量", "需复核", "纠正", "未支持"],
    oldThreadValue: "旧线程能生成报告但易口径漂移；插件应在材料化前先做 claim 和来源边界核准。"
  },
  {
    id: "replay_amount_challenge",
    title: "旧线程 replay: 金额质疑纠偏",
    commandHint: "/analytix ask",
    prompt: "当前案件里有人说合成主体甲转给合成主体乙 4200 万，也有人说 2025-08-27 集中交易约 2100 万；请按两方资金往来核验材料纠偏，不要只用 rank_counterparties 排行作最终结论。请基于当前案件清洗/analysis scope 的只读聚合复核，分列原明细、有效交易/高置信去重、账户/时间集中、交易号风险、支持金额、不可支持金额、重复放大、异常特征、资金意义、暂不能认定事项和下一步补证，明确 4200 万是否能写成事实。",
    expectedTools: ["inspect_case_schema", "run_case_sql"],
    expectedToolBudget: 16,
    expectedMarkers: ["统计期间", "原明细", "高置信", "去重", "账户", "时间集中", "交易号", "重复放大", "异常特征", "资金意义", "暂不能认定", "补证"],
    oldThreadValue: "旧线程能挑战并纠正金额；插件应把金额纠偏变成可回放核验材料。"
  },
  {
    id: "replay_full_case_simple_prompt_runs_tree",
    title: "旧线程 replay: 短句全案分析跑完整树",
    commandHint: "/analytix ask",
    prompt: "对当前案件做全案分析，跑完整功能树，但不要生成正式报告。覆盖已核数据量、时间跨度、清洗/索引覆盖、主体与账户材料覆盖、进出账金额笔数、按人/账户统计、Top20、账户角色、已核异常特征、可证实资金链路、证据缺口和续调清单；不能只给计划。场景专题必须由同案宿主证据回执和匹配的来源能力激活，未激活整段省略，不能把普通全案扩写成固定案类清单。",
    expectedTools: ["run_full_case_analysis"],
    expectedMarkers: ["全案分析", "完整功能树", "数据量", "时间跨度", "清洗", "主体", "账户", "进账", "出账", "账户角色", "Top20", "异常特征", "资金链路", "证据缺口", "续调", "不能只给计划"],
    oldThreadValue: "旧线程需要多轮才跑完全案；插件必须把短句全案请求扩展成完整分析树。"
  },
  {
    id: "replay_source_trace_upstream",
    title: "旧线程 replay: 资金来源上游追溯",
    commandHint: "/analytix ask",
    prompt: "围绕一个可解析账户、主体或交易种子追溯资金来源/上游前手。请输出可见上游、已核交易与待核线索边界、断点、金额/时间差、补调对象；不能写成最终来源或最终权属。",
    expectedTools: ["build_fund_flow_graph", "trace_fund_next_hop"],
    expectedMarkers: ["资金来源", "上游", "前手", "可见上游", "已核交易", "待核线索", "断点", "补调", "最终来源"],
    oldThreadValue: "旧线程会从去向继续追到来源和前手；插件必须支持上游追溯，不只支持下游去向。"
  },
  {
    id: "replay_economic_crime_typology_leads",
    title: "旧线程 replay: 非项目案件专题激活负例",
    commandHint: "/analytix scope",
    prompt: "这是普通交易流水型非项目案件负例：当前仅有同案交易来源的宿主有效回执，没有可激活场景专题的项目、招投标、关系、职务或人工复核来源能力。只写已查范围、交易事实边界、数据缺口和补证动作；任何未激活场景的标题、叙事、表格、图和占位段落都必须省略，不能为了覆盖完整而列举案类。",
    expectedTools: ["get_case_scope_map"],
    expectedMarkers: ["已查范围", "交易", "事实边界", "数据缺口", "补证"],
    scenarioFixture: "non_project_transactions_only",
    forbiddenOutputPatterns: [
      {
        pattern: "(?:工程(?:项目)?|项目款|利益输送|串通投标|围标|行贿|受贿)",
        reason: "non-project case rendered an inactive report scenario"
      }
    ],
    oldThreadValue: "普通全案不得把所有案类词汇当作固定侦查镜头；无匹配 source capability 和 host EvidenceReceipt 时必须整段省略。"
  },
  {
    id: "replay_case_delivery_pack",
    title: "旧线程 replay: 办案交付包",
    commandHint: "/analytix report",
    prompt: "把当前已复核事实组织成办案交付包，而不是正式定稿报告。必须包含事实材料、主体/账户/对手方表、异常特征表、资金流向表、补调材料清单和报告结论复核；不得输出工具日志或内部 gate 模板。",
    expectedTools: ["run_full_case_analysis", "validate_report_claims", "validate_continuation_list"],
    expectedMarkers: ["事实材料", "主体/账户/对手方", "异常特征表", "资金流向表", "补调材料清单", "报告结论复核", "不得输出工具日志"],
    oldThreadValue: "旧线程最终价值是可交付办案材料；插件必须把事实、线索、补证和 QA 组织成材料包。"
  }
];

export const TASKS = buildEvalTasks(BASE_TASKS);

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function loadGoldenAnswerSet() {
  const fixtureUrl = new URL("./eval-fixtures/golden-answer-set.json", import.meta.url);
  try {
    return JSON.parse(fs.readFileSync(fixtureUrl, "utf8"));
  } catch {
    // Golden/oracle facts are eval-only and must not be loaded from production references.
  }
  return { tasks: [] };
}

function loadCapabilityRegistry() {
  const registryUrl = new URL("../references/capability-registry.json", import.meta.url);
  try {
    return JSON.parse(fs.readFileSync(registryUrl, "utf8"));
  } catch {
    return {};
  }
}

function loadEvalCoverageFixture() {
  const fixtureUrl = new URL("./eval-fixtures/eval-coverage-contract.json", import.meta.url);
  try {
    return JSON.parse(fs.readFileSync(fixtureUrl, "utf8"));
  } catch {
    return {};
  }
}

function goldenTaskFor(taskId) {
  const tasks = Array.isArray(GOLDEN_ANSWER_SET?.tasks) ? GOLDEN_ANSWER_SET.tasks : [];
  return tasks.find((item) => text(item.id) === taskId) || null;
}

function goldenCaseFor(caseId) {
  const cases = Array.isArray(GOLDEN_ANSWER_SET?.cases) ? GOLDEN_ANSWER_SET.cases : [];
  return cases.find((item) => text(item.case_id) === text(caseId)) || null;
}

export function reportTaskIdsFromCapabilityRegistry(registry = CAPABILITY_REGISTRY) {
  return registryReportTaskIdsFromCapabilityRegistry(registry);
}

export function isReportTaskId(taskId, registry = CAPABILITY_REGISTRY) {
  return isReportTaskIdFromCapabilityRegistry(taskId, registry);
}

export function expectedAnalytixToolBudgetForTask(taskOrTaskId, registry = CAPABILITY_REGISTRY, options = {}) {
  return registryExpectedAnalytixToolBudgetForTask(taskOrTaskId, registry, options);
}

export function expectedAnalytixToolsForTask(taskOrTaskId, registry = CAPABILITY_REGISTRY, options = {}) {
  return registryExpectedAnalytixToolsForTask(taskOrTaskId, registry, options);
}

function titleForGoldenTask(task) {
  const id = text(task?.id);
  const titles = {
    case_3c72_top_rankings: "3c72 全案 Top 排名",
    case_3c72_report_claim_review: "3c72 既有报告 claim review",
  };
  return titles[id] || text(task?.question || id);
}

function commandHintForGoldenTask(task) {
  const id = text(task?.id);
  if (task?.plugin_expected === false) return "";
  if (/report|claim/iu.test(id)) return "/analytix review";
  if (/top|rank/iu.test(id)) return "/analytix rank";
  return "/analytix ask";
}

function expectedToolsForGoldenTask(task) {
  if (Array.isArray(task?.expected_tools)) {
    return task.expected_tools.map(text).filter(Boolean);
  }
  return registryExpectedAnalytixToolsForTask(task, CAPABILITY_REGISTRY);
}

function evalDimensionsForTask(taskId, registry = CAPABILITY_REGISTRY) {
  const dimensions = Array.isArray(registry?.eval_coverage_contract?.required_dimensions)
    ? registry.eval_coverage_contract.required_dimensions
    : [];
  return dimensions
    .filter((dimension) => Array.isArray(dimension?.task_ids) && dimension.task_ids.map(text).includes(text(taskId)))
    .map((dimension) => text(dimension.id))
    .filter(Boolean);
}

function taskTypeForTask(taskId) {
  return text(TASK_TYPE_BY_TASK_ID[text(taskId)] || "quick_fact");
}

function expectedMarkersWithoutForcedScenarios(taskId, ...markerSets) {
  const id = text(taskId);
  return [...new Set(markerSets.flat().map(text).filter(Boolean))].filter((marker) => {
    if (ORDINARY_FULL_CASE_TASK_IDS.has(id) && ORDINARY_FULL_CASE_FORCED_TYPOLOGY_PATTERN.test(marker)) return false;
    if (id === "full_report_gate" && marker === "必查专题") return false;
    return true;
  });
}

export function buildTaskInventory(tasks = TASKS, registry = CAPABILITY_REGISTRY) {
  return tasks.map((task) => {
    const taskId = text(task.id);
    const goldenTask = goldenTaskFor(taskId);
    const caseId = text(task.case_id || goldenTask?.case_id || DEFAULT_CASE_ID);
    const expectedToolBudget = expectedAnalytixToolBudgetForTask(task, registry);
    return {
      task_id: taskId,
      case_id: caseId,
      case_name: text(task.case_name || goldenCaseFor(caseId)?.case_name),
      title: text(task.title),
      task_type: taskTypeForTask(taskId),
      command_hint: text(task.commandHint),
      plugin_expected: task.pluginExpected !== false,
      report_task: isReportTaskId(taskId, registry),
      passive_nonintervention: passiveNonfundsTaskIdsFromCapabilityRegistry(registry).has(taskId),
      expected_tool_budget: expectedToolBudget,
      expected_tools: expectedAnalytixToolsForTask(task, registry),
      scoring_marker_count: Array.isArray(task.expectedMarkers) ? task.expectedMarkers.length : 0,
      forbidden_claim_count: Array.isArray(goldenTask?.forbidden_claims) ? goldenTask.forbidden_claims.length : 0,
      dimensions: evalDimensionsForTask(taskId, registry),
    };
  });
}

function buildEvalTasks(baseTasks) {
  const tasks = baseTasks.map((task) => {
    const goldenTask = goldenTaskFor(task.id);
    const caseId = text(goldenTask?.case_id || task.case_id || DEFAULT_CASE_ID);
    const caseMeta = goldenCaseFor(caseId);
    return {
      ...task,
      case_id: caseId,
      case_name: text(caseMeta?.case_name || task.case_name),
      expectedMarkers: expectedMarkersWithoutForcedScenarios(
        task.id,
        task.expectedMarkers || [],
        goldenTask?.scoring_markers || []
      ),
    };
  });
  const existing = new Set(tasks.map((task) => task.id));
  const goldenTasks = Array.isArray(GOLDEN_ANSWER_SET?.tasks) ? GOLDEN_ANSWER_SET.tasks : [];
  const replayById = new Map(REPLAY_TASKS.map((task) => [text(task.id), task]).filter(([id]) => id));
  for (const goldenTask of goldenTasks) {
    const id = text(goldenTask.id);
    if (!id || existing.has(id)) continue;
    const replayTask = replayById.get(id);
    if (replayTask) {
      tasks.push({ ...replayTask, case_id: text(replayTask.case_id || goldenTask.case_id || DEFAULT_CASE_ID) });
      existing.add(id);
      continue;
    }
    const caseId = text(goldenTask.case_id || DEFAULT_CASE_ID);
    const caseMeta = goldenCaseFor(caseId);
    const expectedToolBudget = Number(goldenTask.expectedToolBudget || goldenTask.expected_tool_budget);
    tasks.push({
      id,
      case_id: caseId,
      case_name: text(caseMeta?.case_name),
      title: titleForGoldenTask(goldenTask),
      commandHint: commandHintForGoldenTask(goldenTask),
      prompt: text(goldenTask.question),
      expectedTools: expectedToolsForGoldenTask(goldenTask),
      ...(Number.isFinite(expectedToolBudget) && expectedToolBudget >= 0 ? { expectedToolBudget } : {}),
      pluginExpected: goldenTask.plugin_expected !== false,
      expectedMarkers: Array.isArray(goldenTask.scoring_markers) ? goldenTask.scoring_markers : [],
      oldThreadValue: "golden-answer-set 固化题；用于检验插件是否把确定性事实、口径边界和 claim review 落到真实答案。",
    });
    existing.add(id);
  }
  for (const replayTask of REPLAY_TASKS) {
    const id = text(replayTask.id);
    if (!id || existing.has(id)) continue;
    tasks.push({ ...replayTask, case_id: text(replayTask.case_id || DEFAULT_CASE_ID) });
    existing.add(id);
  }
  return tasks;
}

function parseCsv(value) {
  return text(value).split(",").map((item) => item.trim()).filter(Boolean);
}

function parseArgs(argv) {
  const options = {
    backendUrl: DEFAULT_BACKEND_URL,
    caseId: DEFAULT_CASE_ID,
    holderName: DEFAULT_HOLDER_NAME,
    dateStart: DEFAULT_DATE_START,
    timeoutMs: 180_000,
    json: false,
    printPrompts: false,
    listTasks: false,
    fetchFacts: false,
    includeReport: false,
    answersFile: "",
    taskIds: [],
    modeIds: [],
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--backend-url") options.backendUrl = next();
    else if (arg === "--case-id") options.caseId = next();
    else if (arg === "--holder-name") options.holderName = next();
    else if (arg === "--date-start") options.dateStart = next();
    else if (arg === "--timeout-ms") options.timeoutMs = Number(next()) || options.timeoutMs;
    else if (arg === "--answers-file") options.answersFile = next();
    else if (arg === "--tasks") options.taskIds = parseCsv(next());
    else if (arg === "--mode") options.modeIds = parseCsv(next());
    else if (arg === "--json") options.json = true;
    else if (arg === "--print-prompts") options.printPrompts = true;
    else if (arg === "--list-tasks") options.listTasks = true;
    else if (arg === "--fetch-facts") options.fetchFacts = true;
    else if (arg === "--include-report") options.includeReport = true;
    else if (arg === "--all") {
      options.printPrompts = true;
      options.fetchFacts = true;
    } else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  options.backendUrl = text(options.backendUrl).replace(/\/+$/u, "");
  if (!options.printPrompts && !options.listTasks && !options.fetchFacts && !options.answersFile) {
    options.printPrompts = true;
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/model-ab-eval.mjs [options]

Options:
  --print-prompts           Print professional delivery prompt suite.
  --list-tasks              Print compact eval task inventory and registry-derived tool budgets.
  --fetch-facts             Fetch deterministic fact anchors from the Analytix backend.
  --answers-file <file>     Score model answers. JSON may be an array, {answers:[...]}, or {task_id:{mode:answer}}.
  --tasks <ids>             Comma-separated task ids to include.
  --mode <ids>              Comma-separated modes to include in prompt output.
  --all                     Print prompts and fetch facts.
  --backend-url <url>       Backend base URL. Default: ${DEFAULT_BACKEND_URL}
  --case-id <id>            Case id. Default: ${DEFAULT_CASE_ID}
  --holder-name <name>      Main holder/person name. Default: ${DEFAULT_HOLDER_NAME}
  --date-start <date>       Focus start date. Default: ${DEFAULT_DATE_START}
  --include-report          Include run_full_case_analysis in fact anchors. This is slower.
  --timeout-ms <n>          Per-request timeout. Default: 180000.
  --json                    Print JSON.
`);
}

function selectedTasksForOptions(options = {}) {
  const wanted = new Set(arrayOf(options.taskIds).map(text).filter(Boolean));
  return wanted.size ? TASKS.filter((task) => wanted.has(text(task.id))) : TASKS;
}

function selectedModesForOptions(options = {}) {
  const wanted = new Set(arrayOf(options.modeIds).map(text).filter(Boolean));
  return wanted.size ? MODES.filter((mode) => wanted.has(text(mode.id))) : MODES;
}

export function buildPromptSuite(options) {
  const modes = selectedModesForOptions(options);
  return selectedTasksForOptions(options).map((task) => ({
    task_id: task.id,
    case_id: task.case_id || options.caseId,
    case_name: task.case_name || goldenCaseFor(task.case_id || options.caseId)?.case_name || "",
    title: task.title,
    task_type: taskTypeForTask(task.id),
    command_hint: task.commandHint,
    old_thread_value: task.oldThreadValue,
    expected_tools: task.expectedTools,
    plugin_expected: task.pluginExpected !== false,
    expected_markers: task.expectedMarkers,
    prompts: modes.map((mode) => ({
      mode: mode.id,
      label: mode.label,
      prompt: `${mode.instruction}\n\n${task.pluginExpected === false ? "本任务不是涉案资金研判任务；不要调用 analytix_funds、funds_investigate 或 validate_report_claims，也不要读取案件数据。" : ""}\n${buildBaseGuard(options, task)}\n\n任务：${task.prompt}`.replace(/\n{3,}/gu, "\n\n")
    }))
  }));
}

function buildBaseGuard(options, task) {
  const caseId = text(task.case_id || options.caseId || DEFAULT_CASE_ID);
  const caseName = text(task.case_name || goldenCaseFor(caseId)?.case_name || "三文件户名聚合复核-单进程-20260523003351");
  const prompt = text(task.prompt);
  const subjectLine = /合成主体甲|合成主体乙|liuwenliang|zhangjinzhi/iu.test(`${task.id} ${prompt}`)
    ? `主体：${text(options.holderName || DEFAULT_HOLDER_NAME)}`
    : "主体：按题目确定，不预设单一主体";
  const timeLine = /2025|以来|核心日期|核心集中交易|合成主体乙|旧线程|old_thread|outflow/iu.test(`${task.id} ${prompt}`)
    ? `重点时间：${text(options.dateStart || DEFAULT_DATE_START)} 起；同时按题目要求说明完整数据范围/核心窗口差异`
    : "时间范围：按题目确定；题目未限定时间时使用数据实际首末时间，不得自行缩成 2025 年以来窗口";
  return [
    `案件：${caseName} / ${caseId}`,
    subjectLine,
    timeLine,
    "要求：用中文回答；关键数字必须说明统计范围；线索、统计特征、需复核要分开；不要输出法律定性或非法所得最终结论。"
  ].join("\n");
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

async function executeSkill(options, skillId, input = {}) {
  const payload = await postJson(
    `${options.backendUrl}/api/v1/skills/${encodeURIComponent(skillId)}:execute`,
    { input: { ...input, case_id: options.caseId } },
    options.timeoutMs,
  );
  return payload && typeof payload === "object" && Object.prototype.hasOwnProperty.call(payload, "data")
    ? payload.data
    : payload;
}

function safeData(output) {
  return output && typeof output === "object" ? output.data || {} : {};
}

async function fetchFactAnchors(options) {
  const sourceAudit = await executeSkill(options, "audit_unindexed_sources", { table_limit: 200, column_limit: 60 });
  const dataQuality = await executeSkill(options, "audit_case_data_quality", { example_limit: 10 });
  const coverage = await executeSkill(options, "get_scope_coverage", {});
  const scopeCompare = await executeSkill(options, "compare_analysis_scopes", {});
  const accountRank = await executeSkill(options, "rank_accounts", { metric: "turnover", limit: 10 });
  const holderRank = await executeSkill(options, "rank_holders", { metric: "turnover", limit: 10 });
  const counterpartyRank = await executeSkill(options, "rank_counterparties", { metric: "turnover", limit: 10 });
  const sourceToZhangName = await executeSkill(options, "rank_counterparties", {
    holder_name: options.holderName,
    direction_mode: "out",
    metric: "outflow",
    limit: 20,
    dedupe_same_holder_same_fact: true,
    counterparty_group_mode: "name",
  });
  const sourceToZhangAccount = await executeSkill(options, "rank_counterparties", {
    holder_name: options.holderName,
    direction_mode: "out",
    metric: "outflow",
    limit: 50,
    dedupe_same_holder_same_fact: true,
    counterparty_group_mode: "account",
  });
  const ownerScope = await executeSkill(options, "resolve_owner_scope", {
    holder_name: options.holderName,
    include_candidate_accounts: true,
    candidate_min_turnover: 100000,
    limit: 30,
  });
  const duplicateFamilies = await executeSkill(options, "resolve_duplicate_families", {
    holder_name: options.holderName,
    scope_mode: "same_holder_accounts",
    limit: 10,
  });
  const patternProbe = await executeSkill(options, "hypothesis_probe", {
    holder_name: options.holderName,
    include_candidate_accounts: true,
    probe_type: "investigative_patterns",
    keywords: ["合成主体乙", "理财"],
    limit: 20,
  });
  const investigationLab = await executeSkill(options, "run_investigation_lab", {
    holder_name: options.holderName,
    include_candidate_accounts: true,
    analysis_goal: "model_ab_eval old-thread-style suspicious discovery",
    focus_keywords: ["合成主体乙", "理财"],
    date_start: options.dateStart,
    max_hypotheses: 8,
  });
  const topOutflows = await executeSkill(options, "trace_subject_top_outflows", {
    holder_name: options.holderName,
    include_candidate_accounts: true,
    date_start: options.dateStart,
    top_n: 20,
  });
  const missingBusiness = await executeSkill(options, "classify_missing_counterparty_business", {
    holder_name: options.holderName,
    include_candidate_accounts: true,
    missing_kind: "both",
    limit: 20,
  });
  const report = options.includeReport
    ? await executeSkill(options, "run_full_case_analysis", { max_accounts: 6, include_internal_playbooks: true, write_report: false })
    : null;

  const coverageData = safeData(coverage).coverage || {};
  const duplicateSummary = safeData(dataQuality).cleaning_quality?.duplicate_summary || {};
  const duplicateFamilySummary = safeData(duplicateFamilies).summary || {};
  const accountIdentity = safeData(dataQuality).account_identity_quality || {};
  const topAccount = safeData(accountRank).rankings?.[0] || {};
  const topHolder = safeData(holderRank).rankings?.[0] || {};
  const topCounterparty = safeData(counterpartyRank).rankings?.[0] || {};
  const zhangNameRow = (safeData(sourceToZhangName).rankings || []).find((row) =>
    text(row.display_name || row.counterparty_name || row.holder_name) === "合成主体乙"
  ) || {};
  const zhangCoreAccountRow = (safeData(sourceToZhangAccount).rankings || []).find((row) =>
    text(row.counterparty_account || row.account_key || row.display_name).includes("9000000000000000015")
  ) || {};
  const labCards = safeData(investigationLab).hypothesis_cards || [];
  const mandatoryCards = safeData(investigationLab).mandatory_review_cards || [];

  return {
    case_id: options.caseId,
    holder_name: options.holderName,
    fetched_at: new Date().toISOString(),
    expected_summary: {
      txn_total: coverageData.txn_total,
      txn_analyzed: coverageData.txn_analyzed,
      txn_excluded: coverageData.txn_excluded,
      account_count: coverageData.account_count,
      source_file_count: coverageData.source_file_count,
      source_audit_status: safeData(sourceAudit).audit_status,
      potential_unindexed_sources: safeData(sourceAudit).potential_unindexed_sources || [],
      account_dim_unregistered_account_count: accountIdentity.unregistered_account_count,
      same_fact_cross_account_extra_rows: duplicateSummary.same_fact_cross_account_extra_rows,
      same_holder_same_fact_extra_rows: duplicateFamilySummary.same_holder_same_fact?.extra_rows,
      same_holder_same_fact_groups: duplicateFamilySummary.same_holder_same_fact?.group_count,
      full_case_cross_holder_same_fact_groups: duplicateFamilySummary.full_case_review_only?.cross_holder_group_count,
      top_account: topAccount,
      top_holder: topHolder,
      top_counterparty: topCounterparty,
      liuwenliang_to_zhangjinzhi: {
        name_scope: {
          txn_count: zhangNameRow.txn_count,
          outflow_total: zhangNameRow.outflow?.yuan ?? zhangNameRow.outflow_total,
          counterparty_account_count: zhangNameRow.counterparty_account_count,
          largest_date_cluster: zhangNameRow.largest_date_cluster,
        },
        core_account_scope: {
          counterparty_account: zhangCoreAccountRow.counterparty_account || zhangCoreAccountRow.account_key,
          txn_count: zhangCoreAccountRow.txn_count,
          outflow_total: zhangCoreAccountRow.outflow?.yuan ?? zhangCoreAccountRow.outflow_total,
          largest_date_cluster: zhangCoreAccountRow.largest_date_cluster,
        },
        forbidden_duplicate_amount: 42000000,
      },
      owner_scope: {
        direct_account_count: safeData(ownerScope).direct_account_keys?.length || 0,
        candidate_account_count: safeData(ownerScope).candidate_accounts?.length || 0,
        unresolved_high_value_accounts: safeData(ownerScope).unresolved_high_value_accounts || [],
      },
      pattern_probe_findings: safeData(patternProbe).findings?.length || 0,
      lab_cards: labCards.length,
      mandatory_cards: mandatoryCards.map((item) => ({
        hypothesis_id: item.hypothesis_id,
        card_type: item.card_type,
        priority: item.priority,
        evidence_statement: item.evidence_statement,
        claim_guard: item.claim_guard,
      })),
      top_outflows: safeData(topOutflows).top_outflows?.slice(0, 5) || [],
      missing_business_category_summary: safeData(missingBusiness).category_summary || [],
      report_validation: report ? safeData(report).report_validation || {} : null,
      report_write_blocked: report ? Boolean(safeData(report).report?.write_blocked) : null,
    },
    raw_status: {
      source_audit: sourceAudit.status,
      data_quality: dataQuality.status,
      duplicate_families: duplicateFamilies.status,
      coverage: coverage.status,
      scope_compare: scopeCompare.status,
      account_rank: accountRank.status,
      holder_rank: holderRank.status,
      counterparty_rank: counterpartyRank.status,
      owner_scope: ownerScope.status,
      pattern_probe: patternProbe.status,
      investigation_lab: investigationLab.status,
      top_outflows: topOutflows.status,
      missing_business: missingBusiness.status,
      report: report?.status || null,
    }
  };
}

function rawAnswerItems(payload) {
  const source = payload && typeof payload === "object" && !Array.isArray(payload) && Array.isArray(payload.answers)
    ? payload.answers
    : payload;
  return source;
}

function normalizeAnswerRecords(payload) {
  const source = rawAnswerItems(payload);
  if (Array.isArray(source)) {
    return source.map((item) => ({
      task_id: text(item.task_id || item.taskId || item.id),
      case_id: text(item.case_id || item.caseId),
      mode: text(item.mode || "unknown"),
      answer: text(item.answer || item.output || item.text),
      status: text(item.status),
      error: text(item.error),
      infrastructure_error: text(item.infrastructure_error || item.infrastructureError),
      valid_for_scoring: item.valid_for_scoring !== false && item.validForScoring !== false,
      tool_calls: normalizeToolCalls(item.tool_calls || item.toolCalls || item.tool_sequence || item.toolSequence || item.tools || []),
      command_executions: normalizeCommandExecutions(item.command_executions || item.commandExecutions || item.commands || []),
      tool_sequence_analysis: item.tool_sequence_analysis || item.toolSequenceAnalysis || null,
      route_violations: Array.isArray(item.route_violations || item.routeViolations)
        ? [...(item.route_violations || item.routeViolations)]
        : [],
      elapsed_ms: Number(item.elapsed_ms || item.elapsedMs || 0),
      answer_chars: Number(item.answer_chars || item.answerChars || text(item.answer || item.output || item.text).length),
      cost_proxy: item.cost_proxy || item.costProxy || null,
    })).filter((item) => item.task_id);
  }
  const answers = [];
  if (source && typeof source === "object") {
    for (const [taskId, value] of Object.entries(source)) {
      if (typeof value === "string") {
        answers.push({ task_id: taskId, mode: "unknown", answer: value });
      } else if (value && typeof value === "object") {
        for (const [mode, answer] of Object.entries(value)) {
          answers.push({ task_id: taskId, mode, answer: text(answer) });
        }
      }
    }
  }
  return answers.filter((item) => item.task_id);
}

function normalizeAnswers(payload) {
  return normalizeAnswerRecords(payload)
    .filter((item) => item.answer && item.valid_for_scoring !== false && !item.infrastructure_error);
}

function emptyAnswerReason(item) {
  if (item.answer) return "";
  const hasObservedWork = item.tool_calls.length || item.command_executions.length;
  return hasObservedWork
    ? "empty answer"
    : "empty answer with no observed tool or command execution";
}

function invalidAnswerRecords(payload) {
  return normalizeAnswerRecords(payload)
    .filter((item) => item.valid_for_scoring === false || Boolean(item.infrastructure_error) || !item.answer)
    .map((item) => ({
      task_id: item.task_id,
      case_id: item.case_id,
      mode: item.mode,
      status: item.status,
      reason: item.infrastructure_error || emptyAnswerReason(item) || "valid_for_scoring=false",
      route_violations: item.route_violations,
    }));
}

function normalizeToolCalls(value) {
  const items = Array.isArray(value) ? value : [];
  return items
    .map((item) => {
      if (typeof item === "string") return { tool: item, server: "", status: "" };
      if (!item || typeof item !== "object") return null;
      return {
        server: text(item.server || item.server_name || item.serverName),
        tool: text(item.tool || item.name || item.tool_name || item.toolName),
        status: text(item.status),
        duration_ms: Number(item.duration_ms || item.durationMs || 0),
        result_text_chars: Number(item.result_text_chars || item.resultTextChars || 0),
        input: item.input || item.arguments || item.args || null,
      };
    })
    .filter((item) => item && item.tool);
}

function normalizeCommandExecutions(value) {
  const items = Array.isArray(value) ? value : [];
  return items
    .map((item) => {
      if (typeof item === "string") return { command: item, status: "" };
      if (!item || typeof item !== "object") return null;
      return {
        command: text(item.command || item.cmd || item.title),
        status: text(item.status),
        duration_ms: Number(item.duration_ms || item.durationMs || 0),
      };
    })
    .filter((item) => item && item.command);
}

function toolCallsContain(toolCalls, toolName) {
  return normalizeToolCalls(toolCalls).some((item) => item.tool === toolName);
}

function hasAny(answer, markers) {
  return markers.some((marker) => answer.includes(marker));
}

function isBudgetOnlyRouteViolation(value) {
  return /tool budget exceeded: expected at most \d+ analytix_funds tool call\(s\)/iu.test(text(value));
}

function normalizeMarkerText(value) {
  return text(value)
    .toLowerCase()
    .replace(/[,\s，]/gu, "");
}

function markerHit(answer, marker) {
  const source = text(answer);
  const target = text(marker);
  if (!target) return false;
  if (source.includes(target)) return true;
  const aliases = {
    turnover: ["往来总额", "资金流量", "流量"],
    source: ["数据来源", "规范表", "有效流水", "覆盖口径", "规范/可分析交易"],
    规范明细: ["规范表", "有效流水", "规范交易", "规范/可分析交易", "规范入库"],
    direct: ["直接账户", "确定名下账户"],
    directed: ["进/出方向", "进出方向", "具备进/出方向", "有效进/出方向", "有进出方向"],
    candidate: ["待核账户线索", "待核线索", "线索候选"],
    financial_product: ["理财", "基金", "申购", "赎回"],
    同一户名: ["仅按户名", "户名+证件号", "open_name + id_no"],
    "Account Dossier": ["账户资金画像", "账户资金研判", "账户研判", "账户画像"],
    "Subject Dossier": ["主体资金画像", "主体画像", "主体资金研判", "账户基本情况"],
    "Case Context": ["案件上下文卡", "当前案件上下文卡", "案件 Context", "上下文卡"],
    "Case Workbench": ["受控 Case Workbench", "Workbench", "工作台"],
    进账: ["入账", "收入", "流入", "进款"],
    出账: ["支出", "流出", "出款"],
    异常特征: ["可疑特征", "异常/可疑", "风险特征", "异常线索"],
    不能全流水去重: ["不能对跨户名候选做全案盲去重", "不能全案盲去重", "不能做全案盲去重", "不能全案盲目去重", "不能对全案流水做盲目去重", "不能进行全流水盲去重", "禁止全案盲去重", "逐组核验", "不能直接等同为应扣减金额", "不能直接用于扣减全案统计金额"],
    不能写成确认归属: ["不得把未确认线索扩写为已确认归属", "不得写成已确认归属", "不能写成已确认归属", "已确认名下账户", "账户归属事实"],
    确定性资金边: ["确定性交易边", "确定性资金链路", "支持交易边"],
    明日: ["明天", "次日"],
    上午十点: ["上午10点", "上午 10 点", "上午10时", "上午 10 时", "明日上午10点", "明日上午10时", "明日上午 10 时", "明日 10 点", "明日10时", "明日 10 时", "上午10:00", "上午 10:00", "明日上午10:00", "明日 10:00"],
    备注: ["注明", "说明", "备注说明"],
    日期: ["交易日期", "交易时间", "时间字段", "时间范围"],
    星号: ["*", "`*`", "REPT(\"*\"", "REPEAT('*'", "REPEAT(\"*\"", "星号数量"],
    不生成报告: ["未生成报告", "不会生成报告", "不直接生成报告", "不得生成报告", "不写报告", "未写报告", "未进入报告生成", "不是正式报告"],
    不得: ["不能", "不应", "禁止", "不可"],
    validate_continuation_list: ["validate_continuation_list", "后续承接清单", "续查清单", "补调对象", "需补调对象"],
    mandatory_review_cards: ["mandatory_review_cards", "强制复核卡", "强制卡", "报告门禁", "门禁"],
    validate_report_claims: ["validate_report_claims", "报告声明复核", "声明复核", "claim review", "claim 复核", "报告级 claim 复核", "报告级校验", "报告级校验结果"],
    source_audit: ["source_audit", "来源审计", "事实引用", "未绑定事实引用", "待复核草稿"],
    write_blocked: ["write_blocked", "报告级草稿被阻断", "正式报告当前被阻断", "写入阻断", "被阻断", "阻断最终报告", "不能出具正式报告"],
    data_quality: ["data_quality", "数据质量", "质量边界", "数据质量边界", "导入清洗", "双空对手方", "去重/跳过重复"],
    数据质量: ["质量边界", "数据质量边界", "清洗质量", "导入清洗"],
    假设队列: ["复核入口", "复核种子", "复核线索", "优先复核"],
    hypothesis_queue: ["假设队列", "复核入口", "复核种子", "复核线索", "优先复核"],
    hypothesis_status: ["线索支持程度", "已有流水支持", "已证实", "需复核", "需核实", "线索", "降级", "暂不能认定"],
    next_queries: ["next_queries", "下一步查询", "后续查询", "补调", "核验", "复核动作"],
    stop_conditions: ["收口边界", "完成边界", "不能写成", "未取得"],
    时间集中: ["核心窗口", "2025-01-01", "2025 年 1 月"],
    现金断点: ["资金去向断点", "去向断点", "缺失对手大额", "缺失对手"],
    认申购: ["申购", "认购", "购买"],
    不能写成现金去向: ["不能直接写成现金去向", "不能误写成现金流出", "避免误写成现金流出", "避免误写成现金去向"],
    禁止写成事实: ["不能被写成", "不能直接认定", "不得写成", "不得事实化", "不能事实化"],
    未取得确定性资金边: ["未取得 supported transaction edge", "未取得 supported edge", "未返回 supported edge", "没有交易级 supported edge"],
    "unsupported flow": ["unsupported flow", "没有确定性交易边", "没有交易级 supported edge", "未取得确定性资金边", "资金闭环"],
    开放式深挖: ["开放假设", "多主体资金追踪", "不预设单一主体", "可疑通道", "侦查意图", "假设队列"],
    多主体: ["多个主体", "主体、关联公司和关联人", "不预设单一主体", "围绕当前案件主体"],
    核验意见: ["证据缺口", "事实层状态", "证据不足", "未取得", "需复核边界", "当前事实层"],
    证据缺口: ["核验意见", "需复核边界", "证据不足", "未取得", "待补证"],
    时间窗口: ["时间窗", "指定窗口", "指定窗口口径", "时间范围", "起止日期", "date_start/date_end"],
    重算: ["重新核算", "重新计算", "重新跑数", "本轮按", "不沿用", "不复用"],
    未命中: ["未见命中", "没有命中", "未确认已命中", "未返回命中"],
    需复核: ["待复核", "需核实", "补证", "补调"],
    下一跳: ["继续追一层", "二跳交易边", "续查对象", "后续对手", "后续去向", "后续出账"],
    补调: ["补充或重新指定", "补充目标", "补充续查对象", "补证", "补收", "银行回单", "回单", "开户信息", "完整流水"],
    金额统计范围: ["统计范围", "金额主张", "原明细", "raw detail", "有效交易", "高置信", "去重", "金额统计范围", "分列统计", "本轮可支持事实"],
    不能写成事实: ["不能把未支持金额写成事实", "不能直接写成已确认事实金额", "不能写成已确认事实金额", "不能写成已确认金额", "不能把被质疑金额直接写成已确认事实金额"],
    来源边界: ["来源锚点", "事实来源", "未绑定事实来源", "来源审计", "确定性事实来源"],
    纠正: ["修正", "需修正", "纠正口径", "corrected"]
  };
  const targetAliases = aliases[target] || [];
  if (targetAliases.some((alias) => source.includes(alias))) return true;
  const normalizedSource = normalizeMarkerText(source);
  const normalizedTarget = normalizeMarkerText(target);
  return normalizedTarget ? normalizedSource.includes(normalizedTarget) : false;
}

function countHits(answer, markers) {
  return markers.reduce((count, marker) => count + (markerHit(answer, marker) ? 1 : 0), 0);
}

function regexHit(answer, pattern) {
  return pattern.test(text(answer));
}

function specialTaskDimensions(record, context) {
  const {
    answer,
    isPluginMode,
    toolCalls,
    expectedTools,
    markerHits,
    expectedMarkerCount,
    sourceSkepticHits,
    oldThreadHits,
    labelHits,
    usefulnessHits
  } = context;
  const routeHits = isPluginMode
    ? expectedTools.reduce((count, toolName) => count + (toolCallsContain(toolCalls, toolName) || answer.includes(toolName) ? 1 : 0), 0)
    : 0;
  const efficient = isPluginMode && toolCalls.length > 0 && toolCalls.length <= 2 ? 1 : 0;
  if (/^passive_nonfunds_/u.test(record.task_id) && isPluginMode) {
    const markerCoverage = expectedMarkerCount > 0
      ? Math.min(40, Math.round((markerHits / expectedMarkerCount) * 40))
      : Math.min(40, markerHits * 8);
    return {
      no_plugin_route: toolCalls.length === 0 ? 45 : 0,
      general_answer: markerCoverage,
      no_case_fact_leak: /(合成主体甲|合成主体乙|合成主体丙|133011759553CNY0|9000000000000004)/u.test(answer) ? 0 : 15,
    };
  }
  if (record.task_id === "replay_import_clean_coverage" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      import_clean_coverage: Math.min(20, countHits(answer, ["导入", "清洗", "交易文件日志", "规范入库", "原始行", "去重/跳过重复"]) * 4),
      duplicate_risk_boundary: Math.min(25, countHits(answer, ["clean_duplicate=0", "同事实", "换卡", "候选差额", "不能全流水去重", "不能直接"]) * 5),
      identity_and_counterparty_gaps: Math.min(20, countHits(answer, ["空户名", "账户维表", "未登记账户", "对手户名", "对手账号", "双空"]) * 4),
      fact_vs_claim_boundary: Math.min(20, countHits(answer, ["不等于", "不是", "需复核", "线索", "不得", "不能写成事实"]) * 4),
      output_usefulness: Math.min(5, usefulnessHits),
    };
  }
  if (record.task_id === "replay_multisubject_trace" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      open_investigation: Math.min(20, countHits(answer, ["开放式深挖", "多主体", "当前主体", "关联公司", "关联人", "关联", "可疑通道", "线索", "侦查意图", "不预设单一主体"]) * 5),
      verified_leads: Math.min(20, countHits(answer, ["已证实", "双空", "单位纠偏", "重复交易号", "同事实", "理财"]) * 4),
      evidence_boundary: Math.min(25, countHits(answer, ["核验意见", "证据缺口", "支持程度", "不能", "不得", "资金闭环", "未取得交易级确定性资金边", "可证实交易边", "确定性资金边", "需复核"]) * 5),
      next_review_queue: Math.min(15, countHits(answer, ["补调", "下一步", "追查", "回单", "账户映射", "开户信息"]) * 3),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "replay_rescope_recompute" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      rescope_recompute: Math.min(20, countHits(answer, ["重算", "重新", "本轮", "不沿用", "不复用", "指定窗口"]) * 4),
      scope_dimensions: Math.min(25, countHits(answer, ["统计范围", "实际期间", "时间窗口", "核心", "交易号", "账户"]) * 5),
      case_data_grounding: Math.min(15, countHits(answer, ["交易文件", "规范入库", "原始行", "有效流水", "导入去重", "核验材料未返回", "账户维表", "clean_duplicate=0", "同事实"]) * 3),
      boundary_discipline: Math.min(20, countHits(answer, ["不能", "不得", "候选", "需复核", "不直接", "不能擅自补算", "未由核验材料返回", "需明确", "另行重算"]) * 4),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "replay_negative_search" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      search_fields: Math.min(25, countHits(answer, ["户名", "对手", "摘要", "备注", "账号", "交易用途", "证件号", "IP", "MAC", "交易环境", "商户", "联系电话", "住址", "调单", "查控", "casegraph", "aliases"]) * 3),
      negative_boundary: Math.min(25, countHits(answer, ["未命中", "不能", "绝对不存在", "对象级结论", "不能确认某对象是否出现", "字段范围", "本案当前已入库规范数据", "未检出", "本次检索字段", "当前规范数据范围", "当前案件数据边界", "未发现匹配记录", "无法完成", "缺少检索关键词", "不能据此"]) * 5),
      data_scope: Math.min(20, countHits(answer, ["案件", "完整数据范围", "规范入库", "清洗", "清洗字段", "时间范围", "字段", "目标姓名", "不查原始表"]) * 4),
      next_review: Math.min(10, countHits(answer, ["补充目标", "请补充", "目标名称", "具体姓名", "准确写法", "别名", "账号片段", "补充", "逐项检索", "复核字段", "需复核"]) * 2),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "replay_continue_one_more_hop" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      continuation_intent: Math.min(20, countHits(answer, ["一跳", "后续", "下一跳", "二跳", "续查对象", "继续追一层"]) * 4),
      evidence_layers: Math.min(20, countHits(answer, ["确定性资金边", "确定性交易边", "后续对手", "资产转换", "逐笔余额承接"]) * 4),
      boundary_discipline: Math.min(25, countHits(answer, ["不能确认", "不能写成", "不能直接", "缺口", "未返回", "需复核"]) * 5),
      subpoena_or_next_actions: Math.min(15, countHits(answer, ["补调", "补充", "回单", "流水", "重新指定", "下一跳"]) * 3),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "replay_report_materialize" && isPluginMode) {
    return {
      route_and_budget: Math.min(15, routeHits * 6 + efficient * 3),
      report_review_flow: Math.min(20, countHits(answer, ["报告结论复核", "已复核", "未支持", "纠正", "降级"]) * 4),
      source_and_quality_boundaries: Math.min(25, countHits(answer, ["来源边界", "来源审计", "数据质量", "金额统计范围", "来源锚点", "事实来源"]) * 5),
      draft_block_discipline: Math.min(20, countHits(answer, ["需复核", "不能", "正式报告", "待复核草稿", "证据不足", "不能确认"]) * 4),
      flow_guard: Math.min(15, countHits(answer, ["未取得确定性资金边", "确定性资金边", "资金流向图", "资金闭环", "不得画", "未支持"]) * 3),
      tool_efficiency: toolCalls.length <= 2 ? 5 : 0,
    };
  }
  if (record.task_id === "replay_amount_challenge" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      amount_scope_split: Math.min(30, countHits(answer, ["统计期间", "原明细", "高置信", "有效交易", "去重", "时间窗", "核心账户", "交易号", "大额交易"]) * 5),
      duplicate_amplification: Math.min(20, countHits(answer, ["重复放大", "重复风险", "候选重复", "导入阶段去重", "清洗范围", "同事实", "候选差额", "不能直接扣减", "clean_duplicate=0"]) * 4),
      evidence_boundary: Math.min(25, countHits(answer, ["不能写成事实", "证据缺口", "不能确认", "待复核", "事实结论", "不能", "需复核", "未支持", "不能直接"]) * 5),
      recompute_next_steps: Math.min(10, countHits(answer, ["明确", "另行重算", "时间窗", "账户范围", "账户", "交易方向", "去重规则", "核心账户", "交易号范围", "交易号", "起止日期"]) * 2),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 5,
    };
  }
  if (record.task_id === "replay_full_case_simple_prompt_runs_tree" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      full_case_execution: Math.min(25, countHits(answer, ["全案分析", "完整功能树", "完整资金研判树", "run_full_case_analysis", "分析树", "不是计划"]) * 5),
      core_coverage: Math.min(30, countHits(answer, ["数据量", "时间跨度", "清洗覆盖", "材料覆盖", "进账", "出账", "按人", "账户统计", "Top20"]) * 4),
      supported_findings: Math.min(20, countHits(answer, ["已核", "异常特征", "资金链路", "证据缺口", "支持范围", "已查范围", "续调", "补调"]) * 3),
      no_plan_or_report_gate: /只给计划|lane plan|生成正式报告|报告草稿|write_blocked|report gate/iu.test(answer) ? 0 : 20,
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 15,
    };
  }
  if (record.task_id === "replay_source_trace_upstream" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      upstream_intent: Math.min(20, countHits(answer, ["资金来源", "上游", "前手", "入账来源", "可见上游"]) * 4),
      edge_boundaries: Math.min(25, countHits(answer, ["可证实交易边", "线索", "确定性交易边", "待核", "断点", "核验意见"]) * 5),
      amount_time_gap: Math.min(15, countHits(answer, ["金额差", "时间差", "时间窗", "余额承接", "逐笔"]) * 3),
      proof_queue: Math.min(15, countHits(answer, ["补调", "银行回单", "支付机构", "开户资料", "流水", "下一步"]) * 3),
      no_final_origin_overclaim: /(最终来源|最终权属|最终归属|最终受益人|已查明来源)/u.test(answer) ? 0 : 15,
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "replay_economic_crime_typology_leads" && isPluginMode) {
    const renderedInactiveScenario = INACTIVE_REPORT_SCENARIO_PATTERN.test(answer);
    return {
      route_and_budget: Math.min(10, routeHits * 7 + efficient * 3),
      checked_core_scope: Math.min(25, countHits(answer, ["已查范围", "交易", "事实", "时间", "账户", "数据范围"]) * 5),
      evidence_boundary: Math.min(20, countHits(answer, ["事实边界", "证据", "支持", "不能认定", "数据缺口"]) * 4),
      next_proof: Math.min(15, countHits(answer, ["补证", "调取", "权威材料", "下一步"]) * 4),
      inactive_scenarios_omitted: renderedInactiveScenario ? 0 : 30,
      no_legal_typology_overclaim: hasConclusiveLegalOverreach(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "replay_case_delivery_pack" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + (toolCalls.length <= 3 ? 4 : 0)),
      delivery_structure: Math.min(25, countHits(answer, ["事实材料", "主体/账户/对手方", "异常特征表", "资金流向表", "补调材料清单", "报告结论复核"]) * 4),
      evidence_boundaries: Math.min(20, countHits(answer, ["已复核", "未支持", "需复核", "证据缺口", "边界", "降级"]) * 4),
      material_language: Math.min(15, countHits(answer, ["办案交付", "研判材料", "补证", "附件", "事实", "线索"]) * 3),
      no_internal_gate_log: /(工具日志|内部 gate|report gate|write_blocked|diagnostic|doctor|score|artifact_id|q_)/iu.test(answer) ? 0 : 20,
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "top_account" && isPluginMode) {
    return {
      route_and_budget: Math.min(15, routeHits * 8 + efficient * 7),
      coverage_scope: Math.min(18, countHits(answer, ["285243", "540", "285241", "turnover", "规范明细", "同一户名"]) * 3),
      top_account_fact: regexHit(answer, /133011759553CNY0/u) && regexHit(answer, /(2,?215,?741,?657\.05|2215741657\.05)/u) ? 20 : 0,
      metric_separation: Math.min(15, countHits(answer, ["入账", "出账", "笔数", "最大单笔", "不同排序指标"]) * 3),
      holder_boundary: Math.min(17, countHits(answer, ["合成主体甲", "249", "5,524,622,998.78", "户名+证件号", "不能"]) * 3),
      report_usefulness: Math.min(15, usefulnessHits * 2 + sourceSkepticHits + labelHits)
    };
  }
  if (record.task_id === "data_quality" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + efficient * 4),
      import_and_cleaning: Math.min(18, countHits(answer, ["clean_duplicate=0", "555249", "285243", "268608", "规范明细"]) * 4),
      duplicate_risk: Math.min(24, countHits(answer, ["22636", "30878", "316,563,299.79", "同事实", "换卡", "不能全流水去重"]) * 4),
      account_identity_boundary: Math.min(24, countHits(answer, ["487", "159", "158", "1", "90000000000000016", "未登记账户"]) * 4),
      counterparty_gaps: Math.min(12, countHits(answer, ["86633", "47113", "对手户名", "对手账号"]) * 3),
      review_discipline: Math.min(12, countHits(answer, ["不等于", "不能", "需复核", "不能直接", "逐组核验"]) * 3)
    };
  }
  if (record.task_id === "holder_scope" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + efficient * 4),
      direct_scope: Math.min(20, countHits(answer, ["合成主体甲", "249", "登记账户", "待核账户线索"]) * 5),
      amount_facts: Math.min(25, countHits(answer, ["19100", "2,762,771,809.05", "2,761,851,189.73", "5,524,622,998.78", "5524622998.78"]) * 5),
      account_examples: Math.min(15, countHits(answer, ["133011759553CNY0", "9000000000000000006", "9000000000000000000002"]) * 5),
      ownership_boundary: Math.min(20, countHits(answer, ["不能写成确认归属", "不得扩写为已确认归属", "不能把任何候选账户", "已确认归属账户", "同事实", "换卡", "对手户名", "账户归属"]) * 4),
      output_usefulness: Math.min(10, usefulnessHits * 2)
    };
  }
  if (record.task_id === "account_dossier_card" && isPluginMode) {
    return {
      route_and_budget: Math.min(12, routeHits * 5 + efficient * 4),
      dossier_shape: Math.min(20, countHits(answer, ["账户资金画像", "账户画像", "账户基本情况", "资金流入", "资金流出"]) * 4),
      funds_profile: Math.min(24, countHits(answer, ["进账", "出账", "金额", "笔数", "Top 对手", "对手方"]) * 4),
      investigation_value: Math.min(24, countHits(answer, ["异常特征", "资金来源", "资金去向", "联系电话", "住址", "IP", "MAC", "交易环境", "商户", "可疑线索", "证据缺口", "下一步"]) * 3),
      boundary_discipline: Math.min(20, countHits(answer, ["不能", "需复核", "候选", "支持", "边界"]) * 4),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 10,
    };
  }
  if (record.task_id === "subject_dossier_person" && isPluginMode) {
    return {
      route_and_budget: Math.min(12, routeHits * 5 + efficient * 4),
      dossier_shape: Math.min(20, countHits(answer, ["主体资金画像", "账户基本情况", "登记账户清单", "重点账户表", "时间跨度"]) * 4),
      ownership_boundary: Math.min(24, countHits(answer, ["登记账户", "待核账户线索", "不能直接认定", "核验意见", "需补证"]) * 5),
      funds_profile: Math.min(20, countHits(answer, ["资金流入", "资金流出", "重点账户", "重点对手方", "金额", "笔数"]) * 4),
      investigation_value: Math.min(18, countHits(answer, ["异常特征", "资金来源", "资金去向", "可疑用途", "联系电话", "住址", "单位", "法人", "IP", "MAC", "证据缺口", "核查建议"]) * 3),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 6,
    };
  }
  if (record.task_id === "liuwenliang_to_zhangjinzhi" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + efficient * 4),
      complete_data_scope: Math.min(20, countHits(answer, ["25,885,013", "25885013", "18 笔", "核验期间", "完整两方"]) * 4),
      core_cluster_scope: Math.min(20, countHits(answer, ["21,000,000", "2100 万", "5 笔", "2025-08-27", "核心日期集中交易"]) * 4),
      duplicate_guard: Math.min(20, countHits(answer, ["4200 万", "已确认重复", "不作为新增转账", "重复", "放大", "不能"]) * 4),
      account_and_edge_boundary: Math.min(15, countHits(answer, ["9000000000000000015", "9000000000000000018", "9000000000000000009", "确定性交易边", "核心收款卡", "收款核心账号", "核心卡"]) * 3),
      report_usefulness: Math.min(15, usefulnessHits * 2 + labelHits + sourceSkepticHits)
    };
  }
  if (record.task_id === "zhangjinzhi_continuation" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + efficient * 4),
      source_edge: Math.min(18, countHits(answer, ["合成主体甲", "合成主体乙", "21,000,000", "2025-08-27", "5 笔"]) * 4),
      downstream_edges: Math.min(24, countHits(answer, ["浙银理财认申购户", "20,000,000", "2025-08-29", "合成主体丁", "张运喜", "增鑫宝"]) * 4),
      timing_and_amount_clusters: Math.min(16, countHits(answer, ["时间邻近", "金额接近", "核心窗口", "完整数据范围", "13,450,020"]) * 4),
      boundary: Math.min(22, countHits(answer, ["不能直接证明", "不能直接写成", "不能直接认定", "不能确认", "确定性资金边", "逐笔", "余额承接", "早于", "之前"]) * 4),
      continuation_plan: Math.min(10, countHits(answer, ["补调", "核查", "核验", "回单", "流水", "用途", "余额承接", "逐笔"]) * 2)
    };
  }
  if (record.task_id === "outflow_continuation" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + efficient * 4),
      core_window: Math.min(15, countHits(answer, ["2025-01-01", "1043", "62,119,509.05", "6211.950905"]) * 4),
      terminal_classification: Math.min(25, countHits(answer, ["对手字段缺失", "9000000000000000005", "合成主体丁", "合成主体乙", "合成租赁企业甲"]) * 5),
      boundary: Math.min(20, countHits(answer, ["不能写成最终流向", "不能写成最终资金流向", "不能确认", "补调", "未匹配", "需复核"]) * 4),
      continuation_plan: Math.min(20, countHits(answer, ["补调对象", "开户信息", "收款账户流水", "完整流水", "银行回单", "原始银行反馈", "账户归集", "关系材料"]) * 4),
      quality_guards: Math.min(10, countHits(answer, ["同事实", "空户名", "换卡", "不能全流水去重", "不能全流水盲目去重", "全案盲去重"]) * 3)
    };
  }
  if (record.task_id === "full_case_analysis_tree" && isPluginMode) {
    return {
      route_and_budget: Math.min(12, routeHits * 8 + efficient * 4),
      full_case_tree: Math.min(25, countHits(answer, ["全案分析", "数据量", "时间跨度", "清洗覆盖", "材料覆盖", "全案", "分析树"]) * 4),
      scope_stats: Math.min(20, countHits(answer, ["进账", "出账", "金额", "笔数", "按人", "账户统计", "对手方", "Top20"]) * 4),
      supported_findings: Math.min(20, countHits(answer, ["已核", "异常特征", "资金链路", "续调", "补调", "证据缺口", "支持范围", "已查范围"]) * 3),
      no_report_gate: /生成正式报告|报告草稿|write_blocked|report gate/iu.test(answer) ? 0 : 20,
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 15,
    };
  }
  if (record.task_id === "report_builder_generation" && isPluginMode) {
    return {
      route_and_budget: Math.min(15, routeHits * 6 + efficient * 3),
      report_intent: Math.min(15, countHits(answer, ["报告草稿", "正式报告", "材料化", "已验证", "报告生成"]) * 3),
      report_review: Math.min(25, countHits(answer, ["报告结论复核", "复核", "来源边界", "数据质量", "未取得确定性资金边", "未支持", "纠正"]) * 4),
      draft_boundary: Math.min(25, countHits(answer, ["需复核", "补证", "不能", "待复核", "证据缺口", "不能写成"]) * 5),
      report_usefulness: Math.min(20, countHits(answer, ["事实", "统计范围", "边界", "下一步", "附件", "资金流向图"]) * 4),
      tool_efficiency: toolCalls.length <= 2 ? 10 : 0,
    };
  }
  if (record.task_id === "full_report_gate" && isPluginMode) {
    return {
      route_and_budget: Math.min(15, routeHits * 6 + efficient * 3),
      gate_block: Math.min(20, countHits(answer, ["暂缓出具最终报告", "报告结论复核", "报告级校验", "不能出具正式报告", "正式报告级文本暂不出具", "待复核草稿", "未绑定事实", "来源核验", "逐项证据复核"]) * 4),
      verified_facts: Math.min(25, countHits(answer, ["568446", "288482", "285243", "285241", "1,148,585,126.31", "29,093,769.65", "35,975,441.93"]) * 4),
      corrected_claims: Math.min(15, countHits(answer, ["25,885,013", "21,000,000", "4200", "重复放大", "纠正统计范围"]) * 3),
      report_discipline: Math.min(20, countHits(answer, ["不能", "不得", "需复核", "不能写成", "未取得确定性资金边", "证据不足", "未返回可证实交易边"]) * 4),
      tool_efficiency: toolCalls.length <= 2 ? 5 : 0
    };
  }
  if (record.task_id === "case_3c72_top_rankings" && isPluginMode) {
    return {
      route_and_budget: Math.min(15, routeHits * 8 + efficient * 7),
      coverage_scope: Math.min(15, countHits(answer, ["3c72e755b1f2", "422594", "2035188", "428242", "turnover", "open_name + id_no"]) * 3),
      top_account: regexHit(answer, /133011759553CNY0/u) ? 18 : 0,
      top_holder: regexHit(answer, /(合成主体甲|900000000000000002)/u) && regexHit(answer, /(5,?531,?573,?462\.36|5531573462\.36)/u) ? 18 : 0,
      top_counterparty: Math.min(22, [
        /(3,?773,?513,?099\.40|3773513099\.40)/u,
        /(1,?394,?136,?315\.38|1394136315\.38)/u,
        /(60\s*组|open_name\s*\+\s*id_no|仅按户名|不是报告级)/iu
      ].reduce((sum, pattern) => sum + (regexHit(answer, pattern) ? 7 : 0), 0)),
      answer_usefulness: Math.min(12, usefulnessHits * 2 + sourceSkepticHits),
    };
  }
  if (record.task_id === "case_e7e3_top_rankings" && isPluginMode) {
    return {
      route_and_budget: Math.min(15, routeHits * 8 + efficient * 7),
      coverage_scope: Math.min(15, countHits(answer, ["e7e3a55350b4", "49116", "49113", "166", "413,355,095.71", "规范明细"]) * 3),
      top_account: Math.min(20, countHits(answer, ["851006444520001", "合成主体丙", "607", "59,887,270.99", "29,943,687.86", "29,943,583.13"]) * 4),
      top_holder: Math.min(18, countHits(answer, ["900000000000000003", "58", "9,595", "240,706,114.48", "open_name + id_no"]) * 4),
      top_counterparty: Math.min(18, countHits(answer, ["118,231,076.94", "88,498,812.18", "12,117", "双空", "counterparty_name"]) * 4),
      metric_boundary: Math.min(14, countHits(answer, ["turnover", "入账", "出账", "笔数", "最大单笔", "不能", "需复核"]) * 2)
    };
  }
  if (record.task_id === "case_3c72_report_claim_review" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 5 + efficient * 3),
      review_card: Math.min(15, countHits(answer, ["既有报告结论复核", "报告结论复核", "待复核结论", "研判事实"]) * 4),
      correction_or_downgrade: Math.min(20, countHits(answer, ["需纠正", "未支持结论", "未支持/不能确认资金流", "降级", "线索", "需核实"]) * 4),
      boundary: Math.min(25, countHits(answer, ["来源边界", "正式报告暂不出具", "证据不足", "不能确认", "补调", "复核动作"]) * 4),
      flow_guard: Math.min(20, countHits(answer, ["资金流向图", "确定性资金边", "可证实交易边", "不得画", "不能画", "不得写成"]) * 4),
      forbidden: Math.min(10, countHits(answer, ["违法所得", "实际控制", "代持", "最终归属", "不能", "禁止", "禁用表述"]) * 2),
    };
  }
  if (record.task_id === "case_e7e3_quality_boundary" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + efficient * 4),
      import_coverage: Math.min(18, countHits(answer, ["76", "50,700", "49,770", "930", "规范入库", "导入阶段去重"]) * 3),
      duplicate_guard: Math.min(20, countHits(answer, ["clean_duplicate=0", "不等于", "不能", "同事实", "换卡", "不能全流水去重"]) * 4),
      counterparty_gaps: Math.min(18, countHits(answer, ["13,750", "12,037", "对手户名", "对手账号", "双空", "缺失"]) * 3),
      time_boundary: Math.min(16, countHits(answer, ["2004-12-06", "2025-07-09", "早于 1900", "异常早期", "时间范围边界", "旧口径异常时间"]) * 4),
      review_boundary: Math.min(18, countHits(answer, ["需复核", "不能直接", "不能补写", "回单", "原始文件", "口径", "资金去向", "交易时间", "余额", "对手字段", "已证实层级"]) * 3),
    };
  }
  if (record.task_id === "bare_analytix_entry_status") {
    const noInternalLeak = /(doctor|golden|oracle|write_blocked|diagnostic label|report gate|raw rows|大 JSON)/iu.test(answer) ? 0 : 5;
    return {
      route_and_budget: isPluginMode ? Math.min(15, routeHits * 6 + efficient * 3) : 0,
      entry_status: Math.min(20, countHits(answer, ["当前案件", "状态", "active", "案件编号", "案件标识"]) * 4),
      no_report_boundary: Math.min(15, countHits(answer, ["不生成报告", "未生成报告", "报告级", "仅在需要生成或审查报告时使用"]) * 5),
      lane_inventory: Math.min(20, countHits(answer, ["全案分析", "排行", "画像", "资金来源", "资金流出", "去向", "图谱", "补证", "调证", "复核"]) * 3),
      quality_boundaries: Math.min(15, countHits(answer, ["清洗", "口径", "去重", "缺失", "需复核", "候选", "不能直接"]) * 3),
      next_actions: Math.min(10, countHits(answer, ["下一步", "可指定", "建议", "先做", "若"]) * 2),
      no_internal_leak: noInternalLeak
    };
  }
  if (record.task_id === "report_mermaid_legal_guard" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 5 + efficient * 3),
      verified_context: Math.min(15, countHits(answer, ["25,885,013", "21,000,000", "20,000,000", "20,011,946.25", "理财/资产转换线索"]) * 3),
      unsupported_claims: Math.min(25, countHits(answer, ["未支持结论", "未取得确定性资金边", "资金流向图", "违法所得", "不能", "需核实", "确定性资金边"]) * 4),
      source_boundaries: Math.min(15, countHits(answer, ["暂缓出具", "来源核验", "数据质量", "必查专题", "报告结论复核"]) * 3),
      forbidden_phrasing: Math.min(15, countHits(answer, ["已查明", "违法所得", "实际控制", "代持", "最终归属", "禁止", "禁用表述"]) * 3),
      next_review_actions: Math.min(15, countHits(answer, ["产品合同", "认申购", "赎回", "回单", "余额承接", "可证实交易边", "确定性交易边"]) * 3),
      tool_efficiency: toolCalls.length <= 2 ? 5 : 0
    };
  }
  if (record.task_id === "visual_evidence_delivery_pack" && isPluginMode) {
    const visualOverclaimPattern = /(Top|排行|矩阵|热力图|看板|图表)[^。；\n]*(交易路径|资金链路已证实|实际控制|法律结论|犯罪事实)/u;
    return {
      route_and_budget: Math.min(10, routeHits * 5 + efficient * 3),
      delivery_inventory: Math.min(20, countHits(answer, ["证据表格", "Top20", "特征表", "资金流向表", "续调表", "附件目录", "看板", "图表"]) * 3),
      visual_scope_contract: Math.min(20, countHits(answer, ["统计范围", "单位", "时间窗口", "指标", "方向", "支持程度", "时间窗"]) * 2),
      evidence_boundaries: Math.min(20, countHits(answer, ["已复核", "需复核", "线索", "候选", "证据缺口", "不是交易路径", "不得写成", "不能写成"]) * 3),
      visual_form_choice: Math.min(15, countHits(answer, ["表", "排行榜", "柱状", "趋势", "矩阵", "热力", "dashboard", "appendix", "workbook", "附件目录", "看板页", "工作包", "工作簿", "清单", "堆叠"]) * 2),
      next_owner_action: Math.min(10, countHits(answer, ["补调", "续调", "补证", "复核", "下一步", "图谱", "报告复核"]) * 2),
      no_visual_overclaim: hasUnguardedLine(answer, visualOverclaimPattern) ? 0 : 5
    };
  }
  if (record.task_id === "old_thread_patterns" && isPluginMode) {
    return {
      investigation_intent: Math.min(12, countHits(answer, ["研判目标", "历史行为基线", "开放式深挖", "围绕合成主体甲", "围绕主体", "重点复核"]) * 3),
      hypothesis_queue: Math.min(14, countHits(answer, ["假设队列", "合成主体乙", "理财", "缺失对手", "重复交易号", "金额集中"]) * 2),
      evidence_status: Math.min(16, countHits(answer, ["已证实", "已有流水支持", "需复核", "未证实", "支持程度", "核验材料", "高可信线索", "暂不能认定"]) * 3),
      amount_and_temporal_concentrations: Math.min(16, countHits(answer, ["7,958,757.83", "7958757.83", "795.875783", "时间集中", "金额集中"]) * 4),
      duplicate_and_missing_counterparty: Math.min(16, countHits(answer, ["20250103175349102219900882174578", "同事实", "换卡", "缺失对手", "现金断点"]) * 4),
      asset_and_forbidden_guards: Math.min(14, countHits(answer, ["认申购", "赎回", "理财", "不能写成现金去向", "禁止写成事实", "未取得确定性资金边"]) * 3),
      next_queries_and_efficiency: Math.min(12, usefulnessHits * 2 + efficient * 4),
    };
  }
  if (record.task_id === "investigation_lab_cash_asset_hypotheses" && isPluginMode) {
    return {
      route_and_budget: Math.min(10, routeHits * 6 + efficient * 4),
      verified_context: Math.min(18, countHits(answer, ["e7e3a55350b4", "49,116", "49116", "49,113", "49113", "413,355,095.71", "76"]) * 3),
      hypothesis_queue_status: Math.min(20, countHits(answer, ["研判目标", "假设队列", "支持程度", "需复核", "线索", "降级", "暂不能认定"]) * 3),
      cash_break_boundaries: Math.min(20, countHits(answer, ["双空", "12,117", "12117", "88,498,812.18", "88498812.18", "现金断点", "对手方", "补调"]) * 3),
      asset_and_candidate_downgrade: Math.min(20, countHits(answer, ["理财", "资产", "认申购", "赎回", "候选账户", "不能写成", "产品合同", "余额承接"]) * 3),
      next_queries_and_stop_conditions: Math.min(17, countHits(answer, ["下一步查询", "续查动作", "收口边界", "可证实交易边", "回单", "开户资料", "续查清单", "资金流向图", "不得"]) * 3),
      no_final_overclaim: hasConclusiveLegalOverreach(answer) || hasAssetOrCashMisclassification(answer) || hasCandidateOwnershipUpgrade(answer) ? 0 : 5,
    };
  }
  return {
    correct_route_and_gates: Math.min(
      15,
      isPluginMode
        ? Math.max(routeHits * 5, context.allGateHits * 3)
        : Math.min(8, sourceSkepticHits)
    ),
    deterministic_facts: Math.min(30, markerHits * 4),
    source_and_cleaning_skepticism: Math.min(15, sourceSkepticHits * 2),
    old_thread_style_discovery: context.discoveryHeavyTask
      ? Math.min(20, oldThreadHits * 3)
      : Math.min(10, oldThreadHits * 2 + Math.floor(markerHits / 2)),
    claim_discipline: Math.min(15, labelHits * 3),
    tool_efficiency: isPluginMode
      ? (toolCalls.length > 0 && toolCalls.length <= 2 ? 5 : toolCalls.length <= 4 ? 3 : 1)
      : 2,
    output_usefulness: Math.min(10, usefulnessHits * 2),
  };
}

function answerLines(answer) {
  return text(answer).split(/\r?\n|[。；;]/u).map((line) => line.trim()).filter(Boolean);
}

function answerLineEntries(answer) {
  const entries = [];
  let guardedContext = false;
  for (const rawLine of text(answer).split(/\r?\n/u)) {
    const line = rawLine.trim();
    if (!line) continue;
    const guardedHeading = guardedHeadingLine(line);
    if (/^#{1,6}\s/u.test(line)) {
      guardedContext = guardedHeading;
    }
    entries.push({ line, guarded_context: guardedContext });
    if (guardedHeading) continue;
    if (guardedLine(line) || /(不能直接写成|禁用表述|禁止写成|不得写成|不能写成|不可写成|以下.*(?:禁用|禁止|不得|不能))/u.test(line)) {
      guardedContext = true;
      continue;
    }
    if (guardedContext && !/^[-*•]/u.test(line)) {
      guardedContext = false;
    }
  }
  return entries.length
    ? entries
    : answerLines(answer).map((line) => ({ line, guarded_context: false }));
}

function guardedHeadingLine(line) {
  return /^#{1,6}\s/u.test(line)
    && /(未支持|不能确认|需纠正|禁用表述|需复核边界|来源边界|禁止|不得|不可|阻断|unsupported|missing_source_boundaries|forbidden)/iu.test(line);
}

function guardedLine(line) {
  return /(不能|不得|不可|不应|禁止|禁用|避免|不宜|未能|未形成|未取得|未返回|未提供|未绑定|缺少|无法|不能确认|证据不足|需复核|需核实|待核|线索|可疑特征|降级|补调|补证|后再判断|不对|不等于|不是|并非|而非|非直接|不写作|不写成|不作为|不作|不属于|仅作为|只作为|不得写|不能写|unsupported|unsupported_claims|unsupported_flows|missing_source_boundaries|write_blocked|source_audit|data_quality)/iu.test(line);
}

function hasUnguardedLine(answer, matchPattern, guardPattern = null) {
  return answerLineEntries(answer).some(({ line, guarded_context: guardedContext }) => {
    if (!matchPattern.test(line)) return false;
    if (guardedContext) return false;
    if (guardedLine(line)) return false;
    return guardPattern ? !guardPattern.test(line) : true;
  });
}

function hasConclusiveLegalOverreach(answer) {
  const legalPattern = /(已查明|已核实|已确认|认定|确认|坐实)[\s\S]{0,30}(涉黑|黑社会|违法所得|非法所得|犯罪所得|实际控制|代持|最终归属|最终资金归属|最终流向|最终去向|资金闭环)|(涉黑资金|黑社会资金|违法所得|非法所得|犯罪所得|实际控制|代持|最终归属|最终资金归属|最终流向|最终去向|资金闭环)[\s\S]{0,30}(已查明|已核实|已确认|认定|确认|坐实)/u;
  return hasUnguardedLine(answer, legalPattern);
}

function hasCandidateOwnershipUpgrade(answer) {
  const candidateOwnershipPattern = /(候选账户|候选线索|候选卡|candidate)[\s\S]{0,40}(名下账户|名下银行卡|确认归属|已确认归属|确定归属|实际控制|归属事实)|(名下账户|名下银行卡|确认归属|已确认归属|确定归属|实际控制|归属事实)[\s\S]{0,40}(候选账户|候选线索|候选卡|candidate)/iu;
  return hasUnguardedLine(answer, candidateOwnershipPattern, /(只.*线索|线索.*不.*事实|直接账户.*候选|候选.*直接账户)/u);
}

function hasContactAddressOverclaim(answer) {
  const contactAddressPattern = /(同电话|同联系电话|联系电话相同|同住址|住址相同|同地址|同单位|单位相同|同法定代表人|法定代表人相同)[\s\S]{0,50}(实际控制|代持|共犯|共同犯罪|资金流|确定归属|确认归属|名下账户)|(实际控制|代持|共犯|共同犯罪|资金流|确定归属|确认归属|名下账户)[\s\S]{0,50}(同电话|同联系电话|联系电话相同|同住址|住址相同|同地址|同单位|单位相同|同法定代表人|法定代表人相同)/u;
  return hasUnguardedLine(answer, contactAddressPattern, /(只.*线索|仅.*线索|不.*控制|不能.*控制|不可.*控制|不.*代持|不能.*代持|不可.*代持|需复核|待核|不能写成|不得写成|关联线索)/u);
}

function hasDeviceIpMacOverclaim(answer) {
  const devicePattern = /(同IP|同 IP|IP相同|同MAC|同 MAC|MAC相同|同设备|设备相同|同柜员|柜员相同|同网点|网点相同|同商户|商户相同|同终端|终端相同)[\s\S]{0,60}(实际控制|实际操作|操作人|控制人|团伙|共同犯罪|共犯|跑分团伙|同一人|同一物理人)|(实际控制|实际操作|操作人|控制人|团伙|共同犯罪|共犯|跑分团伙|同一人|同一物理人)[\s\S]{0,60}(同IP|同 IP|IP相同|同MAC|同 MAC|MAC相同|同设备|设备相同|同柜员|柜员相同|同网点|网点相同|同商户|商户相同|同终端|终端相同)/u;
  return hasUnguardedLine(answer, devicePattern, /(只.*线索|仅.*线索|不.*证明|不能.*证明|不可.*证明|不能写成|不得写成|需复核|待核|交易环境|关联线索|外部证据|平台记录|设备记录)/u);
}

function hasAssetOrCashMisclassification(answer) {
  const assetCashPattern = /(理财|认申购|申购|赎回|转存|贷款|还款)[\s\S]{0,50}(现金去向|现金流向|最终去向|最终流向|确定流向|确定性资金边|资金闭环)|(现金去向|现金流向|最终去向|最终流向|确定流向|确定性资金边|资金闭环)[\s\S]{0,50}(理财|认申购|申购|赎回|转存|贷款|还款)/u;
  return hasUnguardedLine(answer, assetCashPattern, /(资产转换|资产形态转换|产品凭证|回单|余额承接|补调|需复核|线索|待核|而非|并非|非直接|不写作|不写成)/u);
}

function drawsUnsupportedFlow(answer) {
  return /(Mermaid|flowchart|-->|→)/u.test(answer) && !/(supported|确定性资金边|确定性交易边|工具返回|edge_status|需补调|不能确认|证据不足|不能|不得|禁止|不可|降级|线索)/iu.test(answer);
}

function scoreModeUsesAnalytixMcp(mode) {
  return ["plugin_enabled_passive", "plugin_enabled_commanded", "skill_mcp", "full_plugin"].includes(mode);
}

function isReportLikeTask(taskId, task = {}) {
  return isReportTaskId(taskId) || /report|claim/iu.test(text(taskId)) || /报告|claim/iu.test(`${text(task.title)} ${text(task.prompt)}`);
}

function scoreRecord(record) {
  const task = TASKS.find((item) => item.id === record.task_id) || {};
  const goldenTask = goldenTaskFor(record.task_id);
  const answer = text(record.answer);
  const usesAnalytixMcp = scoreModeUsesAnalytixMcp(record.mode);
  const isPluginMode = usesAnalytixMcp;
  const pluginExpected = goldenTask?.plugin_expected !== false && task.pluginExpected !== false;
  const requiresDirectDataCheck = ["plugin_disabled", "skill_only"].includes(record.mode) && pluginExpected;
  const toolCalls = normalizeToolCalls(record.tool_calls || record.toolCalls || []);
  const toolSequenceAnalysis = objectOf(record.tool_sequence_analysis || record.toolSequenceAnalysis);
  const commandExecutions = normalizeCommandExecutions(record.command_executions || record.commandExecutions || []);
  const routeViolations = Array.isArray(record.route_violations || record.routeViolations)
    ? [...(record.route_violations || record.routeViolations)]
    : [];
  const warnings = [];
  const autoFail = [];

  if (isPluginMode && SOURCE_DELIVERY_FAILURE_PATTERNS.some((pattern) => pattern.test(answer))) {
    autoFail.push("plugin mode turned weak sources, unverified calculations, or missing delivery into production facts");
  }
  const directDataExecutionMarkers = [
    "case.duckdb",
    ".analytix/data-analysis/cases",
    "Library/Application Support/analytix/data-analysis/cases",
    "duckdb.connect",
    "SELECT "
  ];
  if (!isPluginMode && hasAny(answer, directDataExecutionMarkers)) {
    warnings.push("plugin-disabled answer used direct data exploration; compare against source/validation/delivery quality");
  }
  if (requiresDirectDataCheck && commandExecutions.length === 0) {
    autoFail.push(`${record.mode} answer made case-data claims without recorded command execution`);
  }
  const opaqueMarkers = [
    /\bq_[0-9a-f]{8,}\b/iu,
    /\baudit_ref\b/iu,
    /\bdetail_ref\b/iu,
    /\bartifact_id\b/iu,
    /\bevidence_refs\b/iu,
    /证据\s*[:：]\s*q_/iu,
    /审计引用\s*[:：]/iu,
  ];
  if (opaqueMarkers.some((pattern) => pattern.test(answer))) {
    autoFail.push("answer exposed opaque q/audit/artifact refs");
  }
  if (isPluginMode && OUTPUT_DOC_POLLUTION_PATTERNS.some((pattern) => pattern.test(answer))) {
    autoFail.push("plugin answer leaked blueprint/doctor/score/eval internals");
  }
  if (
    isPluginMode &&
    !isReportLikeTask(record.task_id, task) &&
    OUTPUT_REPORT_GATE_POLLUTION_PATTERNS.some((pattern) => pattern.test(answer))
  ) {
    autoFail.push("ordinary investigation answer leaked report-gate/write_blocked wording");
  }
  if (isPluginMode && answer.length > 6000) {
    warnings.push(`plugin answer is too verbose for a low-context fact engine: ${answer.length} chars`);
  }
  const expectedToolBudget = expectedAnalytixToolBudgetForTask(task, CAPABILITY_REGISTRY, { pluginExpected });
  if (isPluginMode && toolCalls.length > 3 && expectedToolBudget <= 3) {
    warnings.push(`plugin mode used ${toolCalls.length} tool calls; ordinary tasks should usually finish in 1-2 calls`);
  }
  const budgetRouteViolations = routeViolations.filter(isBudgetOnlyRouteViolation);
  const hardRouteViolations = routeViolations.filter((item) => !isBudgetOnlyRouteViolation(item));
  if (hardRouteViolations.length) {
    autoFail.push(...hardRouteViolations.map((item) => `route violation: ${text(item)}`));
  }
  if (budgetRouteViolations.length) {
    warnings.push(...budgetRouteViolations.map((item) => `tool budget review: ${text(item)}`));
  }
  const analytixToolCalls = toolCalls.filter((item) => item.server === "analytix_funds" || item.tool === "funds_investigate" || item.tool === "validate_report_claims");
  if (isPluginMode && !pluginExpected && analytixToolCalls.length) {
    autoFail.push(`passive non-funds task over-routed into analytix_funds: ${analytixToolCalls.map((item) => item.tool).join(", ")}`);
  }
  if (isPluginMode && toolSequenceAnalysis.over_budget === true && !budgetRouteViolations.length) {
    warnings.push("tool budget review: tool budget exceeded");
  }
  const repeatedFunds = Array.isArray(toolSequenceAnalysis.repeated_funds_investigate_same_case_task_intent)
    ? toolSequenceAnalysis.repeated_funds_investigate_same_case_task_intent
    : [];
  if (isPluginMode && repeatedFunds.length) {
    autoFail.push(`repeated funds_investigate same case/task/intent: ${repeatedFunds.length}`);
  }
  const hiddenToolCalls = Array.isArray(toolSequenceAnalysis.hidden_tool_calls)
    ? toolSequenceAnalysis.hidden_tool_calls
    : [];
  if (
    isPluginMode
    && hiddenToolCalls.length
    && text(toolSequenceAnalysis.profile || toolSequenceAnalysis.tool_profile) === "frontdoor_report_only"
  ) {
    autoFail.push(`frontdoor profile used hidden analytix_funds tools: ${hiddenToolCalls.map((item) => text(item.tool)).join(", ")}`);
  } else if (isPluginMode && hiddenToolCalls.length) {
    warnings.push(`semantic-toolbox profile used conditional tools: ${hiddenToolCalls.map((item) => text(item.tool)).join(", ")}`);
  }
  const callsAfterFirstFunds = Array.isArray(toolSequenceAnalysis.calls_after_first_funds_investigate)
    ? toolSequenceAnalysis.calls_after_first_funds_investigate
    : [];
  const semanticFollowupsAfterNavigator = callsAfterFirstFunds.filter((item) => /semantic follow-up|continued discovery/iu.test(text(item.reason)));
  if (isPluginMode && toolSequenceAnalysis.budget_limit != null && semanticFollowupsAfterNavigator.length) {
    warnings.push(`continued after navigator card; verify this was a new continuation/rescope/hypothesis: ${semanticFollowupsAfterNavigator.map((item) => text(item.tool)).join(", ")}`);
  }
  if (answer.includes("clean_duplicate=0") && !answer.includes("不等于") && !answer.includes("不能") && !answer.includes("不代表")) {
    autoFail.push("clean_duplicate=0 was not guarded");
  }
  if (isPluginMode && hasConclusiveLegalOverreach(answer)) {
    autoFail.push("legal or final-flow conclusion stated as already verified without evidence boundary");
  }
  if (isPluginMode && hasAssetOrCashMisclassification(answer)) {
    autoFail.push("asset/loan/repayment clue was upgraded to cash destination or final flow");
  }
  if (isPluginMode && hasContactAddressOverclaim(answer)) {
    autoFail.push("contact/address/employer overlap was upgraded to control, ownership, conspiracy, or fund-flow proof");
  }
  if (isPluginMode && hasDeviceIpMacOverclaim(answer)) {
    autoFail.push("device/IP/MAC/transaction-environment overlap was upgraded to operator, control, gang, or co-offending proof");
  }
  if (isPluginMode && pluginExpected && drawsUnsupportedFlow(answer)) {
    autoFail.push("unsupported fund-flow graph or arrow lacked deterministic-edge boundary");
  }
  const taskForbiddenPatterns = Array.isArray(task.forbiddenOutputPatterns) ? task.forbiddenOutputPatterns : [];
  for (const entry of taskForbiddenPatterns) {
    const pattern = text(entry?.pattern || entry);
    if (!pattern) continue;
    let matched = false;
    try {
      matched = new RegExp(pattern, "iu").test(answer);
    } catch {
      matched = answer.includes(pattern);
    }
    if (matched) {
      autoFail.push(text(entry?.reason) || `task-specific forbidden output matched: ${pattern}`);
    }
  }
  if (record.task_id === "liuwenliang_to_zhangjinzhi") {
    if (/(42,?000,?000|4200\s*万)/u.test(answer) && !/(重复|放大|不能|错误|禁止|不应)/u.test(answer)) {
      autoFail.push("42M duplicate-amplified amount was stated without correction");
    }
    const hasFullPeriod = /(25,?885,?013|2588\.5013\s*万)/u.test(answer);
    const hasCoreCluster = /(21,?000,?000|2100\s*万)/u.test(answer);
    if (isPluginMode && (!hasFullPeriod || !hasCoreCluster)) {
      autoFail.push("plugin mode missed 25,885,013 effective pair amount or 21,000,000 core concentrated-transfer distinction");
    }
  }
  if (record.task_id === "zhangjinzhi_continuation" && isPluginMode) {
    if (!/(浙银理财认申购户|理财)/u.test(answer) || !/(20,?000,?000|2000\s*万)/u.test(answer)) {
      autoFail.push("plugin mode missed Zhang Jinzhi downstream 20M wealth-management hop");
    }
    if (/(Mermaid|flowchart|-->|→)/u.test(answer) && !/(确定性|交易边|不得|不能|证据不足)/u.test(answer)) {
      autoFail.push("flow drawing lacked deterministic-edge boundary");
    }
  }
  if (record.task_id === "case_3c72_top_rankings" && isPluginMode) {
    const requiredTopFacts = [
      ["top account 133011759553CNY0", /133011759553CNY0/u],
      ["top holder turnover 5,531,573,462.36", /(5,?531,?573,?462\.36|5531573462\.36)/u],
      ["top counterparty turnover 3,773,513,099.40", /(3,?773,?513,?099\.40|3773513099\.40)/u],
      ["blank-both counterparty turnover 1,394,136,315.38", /(1,?394,?136,?315\.38|1394136315\.38)/u],
    ];
    for (const [label, pattern] of requiredTopFacts) {
      if (!pattern.test(answer)) autoFail.push(`plugin mode missed ${label}`);
    }
  }
  if (record.task_id === "case_3c72_report_claim_review" && isPluginMode) {
    if (!/(未支持|不能确认|证据不足|来源边界|需核实|需补调|降级)/u.test(answer)) {
      autoFail.push("plugin mode missed claim-review unsupported/boundary downgrade");
    }
    if (/(Mermaid|flowchart|-->|→)/u.test(answer) && !/(supported|确定性资金边|工具返回|需补调|不能确认|证据不足|不能|不得|禁止|不可|降级|线索)/iu.test(answer)) {
      autoFail.push("report claim review drew or implied unsupported fund-flow edge");
    }
  }
  const candidateOwnershipGuarded = (
    /(候选账户|候选线索)[\s\S]{0,100}(不得|不能|不可|没有|未确认|不应)[\s\S]{0,100}(确认归属|名下|已确认归属)/u.test(answer) ||
    /(不得|不能|不可|没有|未确认|不应)[\s\S]{0,100}(候选账户|候选线索)[\s\S]{0,100}(确认归属|名下|已确认归属)/u.test(answer)
  );
  const candidateOwnershipLiteralUpgrade = hasUnguardedLine(
    answer,
    /(候选账户|候选线索)[\s\S]{0,80}(名下账户|名下银行卡|确认归属|已确认归属|名下确定)/u
  );
  if (
    (candidateOwnershipLiteralUpgrade && !candidateOwnershipGuarded) ||
    (isPluginMode && hasCandidateOwnershipUpgrade(answer))
  ) {
    autoFail.push("candidate accounts upgraded to ownership facts");
  }
  if (
    (answer.includes("最终流向") || answer.includes("最终去向")) &&
    !/(需复核|补调|不能|不得|未核|不能确认|不能推定|证据不足|边界|不等于)/u.test(answer)
  ) {
    warnings.push("endpoint wording may overclaim final destination");
  }
  const forbiddenClaims = Array.isArray(goldenTask?.forbidden_claims) ? goldenTask.forbidden_claims : [];
  for (const claim of forbiddenClaims) {
    const pattern = text(claim.pattern);
    if (!pattern) continue;
    let matched = false;
    try {
      matched = new RegExp(pattern, "iu").test(answer);
    } catch {
      matched = answer.includes(pattern);
    }
    if (!matched) continue;
    const guard = text(claim.allowed_guard_regex);
    let guarded = false;
    if (guard) {
      try {
        guarded = new RegExp(guard, "iu").test(answer);
      } catch {
        guarded = answer.includes(guard);
      }
    }
    if (!guarded) {
      autoFail.push(claim.reason || `forbidden golden claim matched: ${pattern}`);
    }
  }

  const expectedTools = task.expectedTools || [];
  const expectedMarkers = [
    ...new Set([
      ...(task.expectedMarkers || []),
      ...(Array.isArray(goldenTask?.scoring_markers) ? goldenTask.scoring_markers : [])
    ])
  ];
  const routeHits = isPluginMode
    ? expectedTools.reduce((count, toolName) => count + (toolCallsContain(toolCalls, toolName) || answer.includes(toolName) ? 1 : 0), 0)
    : 0;
  const markerHits = countHits(answer, expectedMarkers);
  const allGateHits = countHits(answer, [
    "audit_unindexed_sources",
    "audit_case_data_quality",
    "get_scope_coverage",
    "plan_case_analysis",
    "run_investigation_lab",
    "validate_report_claims",
    "mandatory_review_cards",
  ]);
  const sourceSkepticHits = countHits(answer, [
    "未纳入口径",
    "规范明细",
    "同事实",
    "换卡",
    "补卡",
    "clean_duplicate=0",
    "158",
    "1",
    "需复核",
  ]);
  const oldThreadHits = countHits(answer, [
    "7958757.83",
    "795.875783",
    "合成主体乙",
    "20250103175349102219900882174578",
    "理财",
    "financial_product",
    "缺失对手",
    "重复交易号",
    "金额集中",
  ]);
  const labelHits = countHits(answer, ["结论", "资金流入", "资金流出", "异常特征", "核验意见", "核查建议"]);
  const usefulnessHits = countHits(answer, ["补调", "下一步", "交易号", "时间", "金额", "账户", "对手", "资金意义"]);

  const discoveryHeavyTask = ["data_quality", "old_thread_patterns", "outflow_continuation", "full_report_gate"].includes(record.task_id);
  const dimensions = specialTaskDimensions(record, {
    answer,
    isPluginMode,
    toolCalls,
    expectedTools,
    markerHits,
    expectedMarkerCount: expectedMarkers.length,
    allGateHits,
    sourceSkepticHits,
    oldThreadHits,
    labelHits,
    usefulnessHits,
    discoveryHeavyTask
  });
  let score = Object.values(dimensions).reduce((sum, value) => sum + value, 0);
  if (autoFail.length) {
    score = Math.min(score, 69);
  }
  score = Math.min(100, score);
  const missingExpectedTools = isPluginMode
    ? expectedTools.filter((tool) => !answer.includes(tool) && !toolCallsContain(toolCalls, tool))
    : [];
  const missingExpectedMarkers = expectedMarkers.filter((marker) => !markerHit(answer, marker));
  const toolResultChars = toolCalls.reduce((sum, item) => sum + Number(item.result_text_chars || 0), 0);
  const checks = {
    data_review_completed: isPluginMode ? (pluginExpected ? toolCalls.length > 0 : toolCalls.length === 0 && answer.length > 0) : commandExecutions.length > 0,
    has_correct_amount_or_key_facts: markerHits >= Math.min(4, Math.max(2, expectedMarkers.length)),
    wrong_scope_intercepted: !autoFail.some((item) => /forbidden|duplicate|候选|clean_duplicate|空户名|report|claim|42M|4200/iu.test(item)),
    mermaid_supported_edge: !/(Mermaid|flowchart|-->|→)/u.test(answer) || /(supported|确定性|交易边|证据|不能|不得|需补调|证据不足)/iu.test(answer),
    candidate_ownership_guarded: !hasCandidateOwnershipUpgrade(answer),
    contact_address_guarded: !hasContactAddressOverclaim(answer),
    device_ipmac_guarded: !hasDeviceIpMacOverclaim(answer),
    asset_cash_boundary_guarded: !hasAssetOrCashMisclassification(answer),
    legal_overreach_blocked: !hasConclusiveLegalOverreach(answer),
    claim_review_blocked: /report|claim|gate/iu.test(record.task_id)
      ? /(validate_report_claims|claim review|阻断|不可报告级|需复核|不能作为|降级|纠正)/iu.test(answer)
      : null,
  };
  const cost = record.cost_proxy || {
    elapsed_ms: Number(record.elapsed_ms || 0),
    answer_chars: Number(record.answer_chars || answer.length),
    tool_calls: toolCalls.length,
    command_executions: commandExecutions.length,
    tool_result_text_chars: toolResultChars,
    context_chars_proxy: Number(record.answer_chars || answer.length) + toolResultChars,
  };
  return {
    task_id: record.task_id,
    case_id: record.case_id || task.case_id || goldenTask?.case_id || "",
    mode: record.mode,
    score,
    grade: score >= 90 ? "publish_grade" : score >= 80 ? "internal_usable" : score >= 70 ? "memo_only" : "fail",
    auto_fail: autoFail,
    warnings,
    dimensions,
    checks,
    cost,
    observed_tool_calls: toolCalls,
    tool_sequence_analysis: toolSequenceAnalysis,
    observed_command_executions: commandExecutions,
    missing_expected_tools: missingExpectedTools,
    missing_expected_markers: missingExpectedMarkers,
  };
}

export function scoreAnswers(answers) {
  const invalidAnswers = invalidAnswerRecords(answers);
  const comparisonRule = "legacy diagnostic only: historical comparison ratings may help inspect regressions, but release quality is now decided by functional closure evidence with source envelope, validation state, delivery state, and current runtime identity.";
  if (invalidAnswers.length) {
    return {
      records_scored: 0,
      by_mode: {},
      results: [],
      invalid_run: true,
      abort_reason: invalidAnswers[0].reason,
      invalid_answers: invalidAnswers,
      comparison_rule: comparisonRule,
    };
  }
  const records = normalizeAnswers(answers);
  if (!records.length) {
    return {
      records_scored: 0,
      by_mode: {},
      results: [],
      invalid_run: true,
      abort_reason: "no scorable answer records",
      invalid_answers: invalidAnswers,
      comparison_rule: comparisonRule,
    };
  }
  const scored = records.map(scoreRecord);
  const byMode = {};
  for (const row of scored) {
    const bucket = byMode[row.mode] || { count: 0, total: 0, auto_fail_count: 0 };
    bucket.count += 1;
    bucket.total += row.score;
    bucket.auto_fail_count += row.auto_fail.length ? 1 : 0;
    byMode[row.mode] = bucket;
  }
  for (const bucket of Object.values(byMode)) {
    bucket.average = bucket.count ? Number((bucket.total / bucket.count).toFixed(2)) : 0;
  }
  return {
    records_scored: scored.length,
    by_mode: byMode,
    results: scored,
    comparison_rule: comparisonRule,
  };
}

function printPromptSuiteText(suite) {
  for (const task of suite) {
    console.log(`\n## ${task.task_id} - ${task.title}`);
    console.log(`command hint: ${task.command_hint}`);
    console.log(`behavior baseline: ${task.old_thread_value}`);
    for (const item of task.prompts) {
      console.log(`\n### ${item.mode} / ${item.label}`);
      console.log(item.prompt);
    }
  }
}

function printTaskInventoryText(inventory, evalCoverage) {
  console.log(formatEvalCoverageSummary(evalCoverage));
  for (const task of inventory) {
    const tools = task.expected_tools.length ? task.expected_tools.join(",") : "none";
    const dimensions = task.dimensions.length ? task.dimensions.join(",") : "unmapped";
    console.log(
      [
        task.task_id,
        task.case_id,
        `budget=${task.expected_tool_budget}`,
        `tools=${tools}`,
        task.passive_nonintervention ? "passive" : "funds",
        task.report_task ? "report" : "non-report",
        `dimensions=${dimensions}`,
      ].join(" | ")
    );
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const evalCoverage = validateEvalCoverageContract({
    capabilityRegistry: CAPABILITY_REGISTRY,
    goldenAnswerSet: GOLDEN_ANSWER_SET
  });
  if (!evalCoverage.ok) {
    throw new Error(`eval coverage contract failed: ${evalCoverage.failures.join("; ")}`);
  }
  const output = {
    case_id: options.caseId,
    holder_name: options.holderName,
    eval_coverage: {
      ...evalCoverage.summary,
      message: formatEvalCoverageSummary(evalCoverage)
    },
    task_inventory: options.listTasks ? buildTaskInventory(selectedTasksForOptions(options)) : undefined,
    prompt_suite: options.printPrompts ? buildPromptSuite(options) : undefined,
    fact_anchors: options.fetchFacts ? await fetchFactAnchors(options) : undefined,
    scoring: undefined,
  };
  if (options.answersFile) {
    const payload = JSON.parse(await import("node:fs").then((fs) => fs.readFileSync(options.answersFile, "utf8")));
    output.scoring = scoreAnswers(payload);
  }
  if (options.json) {
    console.log(JSON.stringify(output, null, 2));
    return;
  }
  if (output.task_inventory) {
    printTaskInventoryText(output.task_inventory, evalCoverage);
  }
  if (output.prompt_suite) {
    printPromptSuiteText(output.prompt_suite);
  }
  if (output.fact_anchors) {
    console.log("\n## deterministic fact anchors");
    console.log(JSON.stringify(output.fact_anchors.expected_summary, null, 2));
  }
  if (output.scoring) {
    console.log("\n## scoring");
    console.log(JSON.stringify(output.scoring, null, 2));
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.stack || error.message : String(error));
    process.exit(1);
  });
}
