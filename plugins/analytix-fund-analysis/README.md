# Analytix 涉案资金研判

这是 Analytix 的**首个旗舰专业插件**，不是 Analytix 主品牌、产品类别或 Core。
它用于证明通用 Agent Platform 能够在同一个 Go Agent Harness 与内置 Privacy
Layer 下进入资金分析、公安情报研判和其他高敏感专业业务。

Analytix 案件项目专用系统插件。当前 deterministic RC candidate 已具备由 Go
宿主动态授权的固定账户流水分析，以及不经过 Agent/MCP/Provider 的本机 typed
display；广泛研判、资金穿透、报告、导出和发布仍处于 fail-closed 边界，不能据此
描述为正式包、商业许可或已发布能力。

> 当前安全状态（优先于下方历史版本记录）：普通插件 `.mcp.json` 继续固定
> `disabled: true`；只有 Go 宿主可以在当前案件、不可变快照、来源、执行和证据
> authority 全部有效时广告 `analyze_account_flows`，`count_case_rows` 只作宿主
> 内部 canary。EvidenceReceipt → ClaimRecord → Final Evidence Gate 只允许发布受
> 支持的 inflow、outflow、count、coverage/currentness，并从已 gate 的金额 claim
> 推导 net；证据不足的 counterparty 必须省略或显示 gap。完整 PII 只可到可信
> 本机 typed sink。`run_full_case_analysis`、报告、导出和广泛 dormant 工具仍不
> 广告、不执行；Focused/cross-layer PASS 不等于 formal package 或发布证明。

## analytix 适配边界

- 插件源码已迁移到 analytix 仓库的 `plugins/analytix-fund-analysis`，旧 Downloads/Analytix_new 目录只作为历史来源，不再作为开发或发布源。
- 生产数据面由 Main/Go 根据当前 main frame、active case 和 current immutable
  dataset snapshot 解析；renderer、模型和工具调用方不提供 DuckDB 路径、SQL、
  `threadId`、`turnId` 或内部 `entityRef` 来启动 Direct Source Preview。
- 插件不再使用 Analytix_new 的“案件管理”全局选中案件、旧 `~/.Analytix资金分析工具/cases`、`ANALYTIX_FUNDS_ACTIVE_CASE_ID` 或后端 `/cases/active` 作为案件来源。
- Agent 固定工具只接受宿主解析的稳定 subject reference 与有界时间/证据行参数；
  ordinary MCP 配置、provider 参数或 renderer 请求不能选择案件、快照、路径或
  数据库。
- `export_cleaned_case_data` 和 data-analysis 的 raw/cleaned/stats/case-archive
  导出不得把 renderer 路径传给 Python，也不得创建 job、目录或文件；后续只能
  在独立接受的 staging、授权、receipt/hash/case/snapshot/PII 和原子发布契约下
  重新开放。

## 当前生产运行结构

- `.codex-plugin/plugin.json`：插件 manifest 和用户展示信息。
- `.mcp.json` 当前固定 `disabled: true`。其受控 JavaScript 协议闭包只声明
  `count_case_rows` 与 `analyze_account_flows` 两个 `taskSupport=forbidden` 的固定
  host-capture schema：它不读取案件源、不形成 provider catalog，也不能执行
  account-flow 分析。Go runtime 的 reserved、pinned、host-owned
  `analytix_funds` in-process binding 是独立生产入口，普通配置不能伪造或覆盖。
- `scripts/production-mcp-entry-closure.json` 是当前桌面包和 Hub 包允许装入的精确生产闭包。`case-project-context.mjs`、backend client、DuckDB/workbench、casegraph、fact compiler、report writer、notebook 和 export 等其他模块目前均为 source-only、dormant 或 test-only 目标实现，不属于生产可达路径，不得以文件存在推断能力已经开放。
- Go 宿主在每次广告与执行前重验 current-run authority；当前 provider-visible
  工具仅为 `analyze_account_flows`，`count_case_rows` 保留为内部 canary。工具
  revoke、来源变化或 authority 失效会移除该分析工具且不恢复 JavaScript
  fallback；正式 artifact、报告和外部发布仍有独立 gate。不得把旧模块直接加入
  闭包或把 cached catalog 当作在线证明。
- `skills/`：Data Analytics 式多 skill 产品面。`analytix-fund-analysis` 保留兼容入口，`index` 做轻量路由，focused skills 覆盖当前案件上下文、数据质量与口径、资金事实快查、特定双方资金往来核验、账户/主体画像、对手方、资金追踪、开放式侦查实验室、全案分析、报告、补调取证、研判完整性复核、报告事实结论复核、研判材料交付复核、资金流向图谱、视觉证据和受控 Case Workbench。
- `references/`：发布校验和脚本使用的根参考副本；完整 `/analytix ...` 命令表位于 `references/command-router.md`，机器可读路由位于 `references/command-metadata.json`，最小 capability registry 位于 `references/capability-registry.json` / `references/capability-registry.schema.json`，顶级插件化总方案位于 `references/top-pluginization-plan.md`，并声明 eval 覆盖门禁与 passive 非资金误触发题；Hub 生命周期位于 `references/hub-lifecycle.md`，反模式位于 `references/anti-patterns.md`。这些文件必须与 `skills/analytix-fund-analysis/references/` 保持一致，不包含本机路径、会话回归、真实案件样例、golden/oracle 数据或研发测试材料；eval rubric 和 golden/oracle 只允许留在本地 eval-only 脚本材料中。
- `scripts/prepare-hub-package.mjs`：只在 `/tmp` 下生成可复核的 Hub 包源、marketplace stub 和 tar.gz 摘要；它不是 Hub publish/install/upgrade/remount 路径，不能写 Analytix runtime 或系统 Codex 状态。
- `output/analytix-fund-analysis/functional-closure/`：当前候选版本的真实功能闭环证据目录。该目录只作本地验收输出，不进入发布包。插件源码目录下不再保留历史 `evidence/` A/B 产物，Hub 包源也会排除旧产物目录，避免旧证据、旧禁令或旧 golden 影响当前发布判断。
- `assets/`：插件图标与 logo。

## 开发启动

当前受支持的生产入口是 Go Host 准入的 `host-static-first-party` 包；必须同时
核验包内容、物化/激活、当前身份和案件/快照 authority，源码目录存在不等于已安装。
普通 `.mcp.json` 保持 disabled。桌面远端 archive 安装仍拒绝缺少 Go archive
authority 的请求，不能将本段作为 Hub 已发布或可安装的证明。

以下历史开发入口不注册、安装或启用本地 overlay；运行前遵循仓库开发 runbook
的隔离 profile、存储和凭据准入，不用它绕过正常启动前提：

```bash
cd <repo-root>
node plugins/analytix-fund-analysis/scripts/dev-run-with-plugin.mjs
```

第一方 Funds 的发现、启用、调用、禁用和重开须通过当前候选的实际封包/Host
路径验收；任意 Hub archive、完整研判、报告及导出仍是后续完整产品范围。

## 后续产品命令入口（当前 provider catalog 不广告）

以下命令描述广泛产品路由，不代表当前生产 provider catalog 已开放。当前只有
Go 宿主按 authority 动态广告固定 `analyze_account_flows`；其他命令只能返回能力
缺口、已检查范围和补证建议，不得由模型模拟工具、拼接 dormant 模块或从历史
输出补造案件事实。

用户可直接说“全案分析”，也可以使用命令式提示：

- `/analytix`：入口/状态命令，解析当前案件项目并给出可选研判 workflow；不生成报告。
- `/analytix report`：在用户明确要求报告、材料、附件或正式文书时，生成或更新当前案件项目资金研判材料并执行 claim/list 复核。
- `/analytix map`：先生成低上下文数据/口径图谱，展示导入、清洗、规范明细、分析索引、未纳入口径来源、数据质量和报告边界的节点/边与必过 gate。
- `/analytix scope`：查看导入、清洗、统计口径、全案一致性和范围覆盖率。
- `/analytix account <账户号>`：解析卡号/账号后执行账户全量分析。
- `/analytix holder <户名>`：解析户名账户集合后执行集合级全量分析。
- `/analytix discovery`：大额、夜间、快进快出、拆分、现金、理财、工程、涉诉、资产等开放式线索。
- `/analytix lab`：开放式深挖可疑点，先输出少量事实卡再继续查证；每张卡带 `priority`、`fact_statement`/`claim_guard` 等字段，并返回 `mandatory_review_cards` 防止漏掉高价值种子。
- `/analytix cash`：现金存取、户名缺失、柜面/ATM 线索。
- `/analytix outflows`：主体/户名/账户集合 Top 出账、终点分类和补调清单。
- `/analytix amount`：A 转 B 多少钱、金额范围争议、重复/换卡/同事实风险的特定双方资金往来核验；排行、入口卡片和专项资金核算只作支撑证据。
- `/analytix cash-bridge`：同日/次日取现-存现对应特征。
- `/analytix payment`：三方支付通道出入金。
- `/analytix crypto`：虚拟资产购置或出入金线索。
- `/analytix path`：资金链路、下一跳、回流、闭环。
- `/analytix role`：生活卡、中转卡、资金归集/沉淀账户、个人账户经营性使用。
- `/analytix asset`：购车、购房、理财、保险、借贷、物业停车等资产债务线索。
- `/analytix control`：代持、实际控制、共用设备/IP/MAC/手机号等线索。
- `/analytix evidence`：补调账户、银行、支付机构和材料清单。
- `/analytix visual`：把已复核事实整理成证据表格、Top20 图表、看板卡和附件工作包。
- `/analytix sql`：仅在现有语义工具不足以覆盖明确自定义口径时，进入受控 Case SQL；当前案件、只读、清洗表/分析索引、限行、purpose、可回放，并返回证据边界和验证状态。
- `/analytix notebook`：为明确可回放分析需求创建受控 Case Notebook/Workbench 记录；结果仍需按证据边界交给 visual-evidence、analysis-critique、claim-review 或 report-builder。
- `/analytix pattern`：不规律模式深挖，复核重复交易号、缺失对手大额、金额集中、人名/理财关键词链路。
- `/analytix plan`：评估当前案件复杂度、是否需要子代理、并给出执行计划。
- `/analytix qa`：对报告数字、口径、措辞、附件空字段、重复交易和金额单位做复核。
- `/analytix qc`：检查输出是否具备结论、依据、异常、案件意义、不能认定事项和补证动作，避免工程词或模板化文本进入经侦材料。
- `/analytix ask`：自然语言 navigator；问题模糊且最佳语义工具不明确时才优先走 `funds_investigate`，避免把完整工具菜单变成用户回答。
- `/analytix casegraph`：返回 Analytix casegraph v1 的口径图谱、质量门、Top 实体和下一步工具建议。
- `/analytix flowgraph`：返回确定性资金流向图，只有端点、时间、金额、方向均可核验的交易链路才能进入图；普通用户看到材料式图、链路表和补证边界，不展示 Mermaid 源码或内部审计编号。

## 数据边界

当前案件事实只能通过 Go 宿主固定的 `analytix_funds` 数据面形成 source-backed
结果。宿主负责 active case、不可变 snapshot、权限、当前性、query/evidence
lineage、证据 carrier 和内部审计；Agent 可见结果不包含完整 PII、路径、SQL 或
内部审计编号。Direct Source Preview 读取 source-exact typed rows，但完全位于
Agent/MCP/evidence 链外；AcceptedSlotDisplay 只有在 exact receipt/claim/final gate
匹配后才解析原绑定 snapshot。`full|masked` 只改变最终本机 projection，不改变
retained source、ClaimRecord、EvidenceReceipt 或模型可见历史。广泛语义工具、
Workbench、报告和外部效果仍是后续独立范围。

当前未版本化 deterministic RC candidate 在 0.16.16 的来源固定基础上新增
`analyze_account_flows` 的 exact typed evidence → ClaimRecord → Final Evidence
Gate 正向链、host-private evidence carrier 的 exactly-once consume/discard、
Direct Source Preview 和 retained AcceptedSlotDisplay。该状态尚未改变插件版本、
license 或发布身份，也不代表 formal package 已通过。

0.16.16 已取代 0.16.15；该候选版本将 current-run native source probe 与精确交易索引计数绑定到同一只读 DuckDB 事务和确定性 dataset snapshot，并要求 Go 宿主在高风险 probe 与执行前校验完整安装态插件源码树。计数仍保持 `safeToAnswer=false`，只有宿主完成 ExecutionGrant、两阶段 EvidenceReceipt 落账和 Final Evidence Gate 后才可发布案件事实。0.16.16 尚不能仅凭版本号或本地 contract 测试视为已发布。

0.16.15 已被 0.16.16 取代；该版本不改变 0.16.14 的资金研判运行时语义，但把 Go-only runtime P0/P1 收口、ModelExecutionRef provider/model 硬绑定、goal/todo final-readiness、release-slice 分组与当时的插件候选身份收口到同一可标记源码。

0.16.14 已被 0.16.15 取代；该版本不改变 0.16.13 的运行时取数和前门资金流向修复，但把发布身份补齐为干净、可复核的当前源码：`release-slice-audit.mjs` 对当前 runtime/renderer 工作树脏改分类覆盖已提交，避免把未提交发布脚手架混进 Hub 源和安装态证据。

0.16.13 已被 0.16.14 取代；该版本修复真实前门资金流向口径 P0：用户说“江苏航案件分析中资金流向前十位对手方是谁”时，`江苏航案件分析中` 表示当前案件项目范围，`资金流向前十位` 表示全案出账去向，不得被模型或工具误拆成 `holder_name=江苏航`，也不得答成不区分方向的 `turnover`。排行 runtime 现在会在 `holder_name` 只是当前案件项目名片段时移除主体过滤，并在当前 turn 用户原话包含资金流向/去向/转给谁/付款对象/收款方前 N 时强制 `rank_counterparties` 使用 `metric=outflow`、`direction_mode=out`、户名粒度返回，由 `frontdoor-p0-oracle` 对照 DuckDB 全案外部出账 Top10 回归验证。0.16.13 同时保留 0.16.12 的案件名范围 guard、0.16.11 的户名粒度默认、空户名/同名自转过滤、Top-N 内容核对、Pair Amount source-of-truth、Workbench ladder 和 L5 前门 gate。

0.16.12 已被 0.16.13 取代；该版本修复真实前门案件名范围 P0：用户说“江苏航案件分析中资金流向前十位对手方是谁”时，`江苏航案件分析中` 表示当前案件项目范围，不得被模型或工具误拆成 `holder_name=江苏航`。排行 runtime 会在 `holder_name` 只是当前案件项目名片段时移除主体过滤，按全案对手方 TopN 返回；但真实前门复核继续暴露“资金流向”可能被模型选成双向交易总额，因此不得作为最终无 P0 版本。

0.16.11 已被 0.16.12 取代；该版本修复真实前门 Top-N 对手方粒度 P0：自然语言问“谁/户名/对手方/资金去向前 N 位”时，`rank_counterparties` 默认按解析户名聚合，并排除空户名和同名自转后补足请求的 Top N；只有用户明确要求账号、卡号、账户粒度、空户名账号核验或同名自转/内部调拨时才切换相应口径。`frontdoor-p0-oracle` Task C 现在逐位核对 DuckDB 户名级 Top10 的名称和金额，`mcp-return-oracle` 也覆盖未显式传粒度的默认路径，避免只检查可见 10 行却内容口径错误。

0.16.10 已被 0.16.11 取代；该版本在 0.16.9 的真实前门 P0 闭环基础上补齐成熟插件结构门禁：重新对照 Data Analytics、Investment Banking、Public Equity Investing、OpenAI Codex Skills/Plugins/MCP 和 Anthropic Agent Skills / skill-creator，新增 `scripts/skill-creator-alignment-audit.mjs`，把 focused skill 必须短、触发描述必须清楚、router 只选 owner、lead workflow 承担交付、support 层不得泄漏、报告/图表/notebook 必须有真实 artifact 检查、生产 skill 不得写入 golden/固定案件事实等要求变成可重复 L0 gate。0.16.10 不改变 0.16.9 已核验的 DuckDB 事实层逻辑，但真实前门复核暴露普通对手方 Top-N 仍可能落到账号粒度，因此不得作为最终无 P0 版本。

0.16.9 在 0.16.8 的真实前门 P0 闭环基础上修复剩余发布风险：Analytix runtime 在案件项目资金问题中会把 provider-visible 工具收敛到 analytix-fund-analysis owner，保留 MCP `structuredContent` 与 compact text，报告/简报请求在后端报告服务不可达或未返回文件时自动落到当前案件 DuckDB 本地 artifact 生成并检查文件签名；`frontdoor-p0-oracle` 的 L5 review 现在是硬门禁，报告/附件必须真实存在且已检查，缺失数据不得误判成 0。0.16.9 仍必须以安装态 `frontdoor-p0-oracle --fail-on-gaps` 0 failed、L5 review、runtime cache equality 和 Hub 发布证据作为发布门禁，不能用 0.16.8 或 direct MCP 产物替代。

0.16.8 已被 0.16.9 取代；该版本在 0.16.7 的真实前门 oracle 失败复盘基础上修复剩余 P0 证据面：Workbench/专项资金核算 compact text 现在把当前案件只读策略、DuckDB EXPLAIN parser/binder 预检、证据卡摘要、source hash、metric scope 和验证摘要写入模型可见的工具结果；`frontdoor-p0-oracle` 同时兼容 Go runtime `tool_result.output.result` 只保留 compact text 的真实事件形态，并把“总流水/周转额/流入金额与流出金额之和”识别为户名排行默认口径。0.16.8 仍必须以安装态 `frontdoor-p0-oracle --fail-on-gaps` 0 failed、L5 review、runtime cache equality 和 Hub 发布证据作为发布门禁，不能用 0.16.7 或 direct MCP 产物替代。

0.16.7 已被 0.16.8 取代；该版本在 0.16.6 的真实前门 canary 基础上修复三个 P0 失败面：`count_case_rows` 对无过滤的交易明细总量统一规范到 `analysis_txn_detail_idx`，避免“交易明细表数据量”落到宽表/规范表行数；`investigate_pair_amount` 明确接受 `counterparty_name`、`payee_name` 等自然语言别名并归一化到 `receiver_name`，同时继续拒绝未声明字段和 per-call 数据源；`frontdoor-p0-oracle` 改为任务级精确匹配 Top 10 去向与 2024 每月 Top-N 自定义口径，并支持元/万元/亿元金额可见答案核对。

0.16.6 已被 0.16.7 取代；该版本在 0.16.5 基础上把剩余 P0 变成前门可审计门禁：新增 `scripts/frontdoor-p0-oracle.mjs`，读取真实 analytix 案件项目线程的 `events.jsonl` / `messages.jsonl`，抽取用户问题、助手最终答案、MCP tool args/results、SQL policy、evidence card、validation state、可见数字、Top-N 行数和内部词泄漏，并用当前案件 DuckDB 做交叉核验。MCP request handler 现在按工具 schema 拒绝未知字段，禁止 per-call 数据源/连接串，保留 Analytix runtime 注入的当前 workspace 上下文；排行、追踪和图谱 fallback 输出统一携带 Top-N contract；语义工具不足时返回结构化 Workbench ladder、禁止声明和 `safe_to_answer_current_task=false`，模型必须继续看当前案件 DuckDB 现场或停为能力缺口。

0.16.5 已被 0.16.6 取代；该版本把 DuckDB Workbench 的数据库现场能力继续收紧到 `crystaldba/postgres-mcp` restricted-mode 方向：用户可控 SQL 在执行前必须先通过静态只读/清洗范围 guard，再通过 DuckDB `EXPLAIN` parser/binder 预检；`run_case_sql`、`explain_case_sql`、`diagnose_case_sql`、`preview_case_rows` 和 notebook cell 都会返回结构化 `sql_policy`，并写入 `validation_state` / evidence card / history 摘要。预检只运行 `EXPLAIN <redacted-current-case-sql>`，不会执行用户 SQL；真正执行只发生在 parser/binder 通过之后。`mcp-return-oracle` 已新增真实 DuckDB 门禁：缺失 parser/binder policy 或非法字段未在数据返回前阻断都会失败。

0.16.4 已被 0.16.5 取代；该版本修复 MCP Top-N 和 DuckDB Workbench 诊断闭环：`rank_counterparties(limit=10)` 等排行工具的 Agent 可见文本、key facts 和 Workbench preview 必须按用户请求的有界 Top N 返回，不再固定截断为 5；`funds_investigate(intent=destination, top_n=10)` 会先抓取带 buffer 的排行行，再排除同主体/自转对手方，确保仍输出 10 个外部去向事实；`trace_subject_top_outflows` 的 `top_outflows` key facts 也按 `top_n` / `limit` 保留，不再被 4/6/8 这类预览常量截断；前门会保留显式 `metric` 和 `counterparty_group_mode`，并把“给谁/转给谁/流向谁”识别为对手方排行/去向问题；`diagnose_case_sql` 在字段名错误时返回缺失字段、DuckDB candidate bindings、SQL-safe candidate columns 和 `analysis_txn_detail_idx` 修复模板，并保证 `account_open_name`、`counterparty_name`、`dc_val`、`amount` 等 SQL identifier 不被用户正文翻译层改写；`validate_report_claims` 在可选后端语义服务不可达时仍执行本地确定性风险复核，并标记 backend capability gap。对齐 `crystaldba/postgres-mcp` 的数据库现场能力时，本版不是开放任意本机 DuckDB，而是在当前案件隔离内提供 schema/profile/count/EXPLAIN/diagnose/run_sql/history/notebook，同时把 `trace_subject_top_outflows`、`trace_fund_next_hop`、`trace_fund` 和 `build_fund_flow_graph` 接入当前案件 DuckDB 本地 fallback：后端语义服务不可达或图谱 runtime 少注入时，仍返回一跳 Top 出账、终点分类、案内下一跳候选、图谱边/断点、query id 和明确穿透深度/数据断点。`mcp-return-oracle` 新增真实 DuckDB Top 10 可见结果、destination Top 10 过滤、`top_outflows` requested Top-N 保留、trace fallback Top 10、next-hop fallback、graph fallback 和 SQL-safe 诊断文本门禁。

0.16.3 已被 0.16.4 取代；该版本完成 final analytix-only runtime cleanup，默认 Hub cache、case DB audit、artifact-store 诊断和安装版 MCP 取数核验均切到当前 analytix 案件项目机制。

0.16.2 已被 0.16.3 取代；该版本补齐当前案件只读 `create_case_notebook`，让 Workbench 能生成可回放 notebook artifact。

0.16.1 在 0.16.0 基础上修正当前案件确认链路：当 `.analytix/case-project.json` 绑定的 DuckDB 已存在时，`get_current_case` 可直接返回案件项目已就绪卡片，不再因为可选 backend case API 临时不可达而阻断后续 DuckDB 统计；显式 `case_id` 跨案件保护和 DuckDB 缺失阻断保持不变。

0.16.0 已被 0.16.1 取代；该版本在 0.15.131 基础上完成 analytix-only 迁移：源码、skills、references、脚本和资源进入 analytix 仓库；MCP 案件来源改为 runtime workspace -> `.analytix/case-project.json` -> data-analysis DuckDB；显式 `case_id` 必须与案件项目绑定一致；本地 Workbench 和清洗导出验收不再使用 Analytix_new/旧资金工具目录。

0.15.131 已被 0.16.0 取代；该版本在 0.15.130 基础上修正插件页视觉资产角色：`assets/logo.png` 不再是带 `Analytix / Fund Analysis` 文案的横幅卡片，而是与 `assets/icon.png` 相同的方形纯图标，插件页由界面自行显示标题和副标题，形态对齐 Computer Use 插件的左图标、右文字展示方式；本版不改变当前案件资金研判行为、MCP 工具、focused skill 路由或证据合同。

0.15.130 已被 0.15.131 取代；该版本在 0.15.129 基础上替换资金分析插件发布视觉身份：`assets/icon.png` 和 `assets/logo.png` 统一使用新的 Analytix 玻璃风格标识，供 Analytix Hub 列表、插件详情页和 composer 图标展示；本版不改变当前案件资金研判行为、MCP 工具、focused skill 路由或证据合同。

0.15.129 已被 0.15.130 取代；该版本在 0.15.128 基础上完成资金分析插件发布面命名复核和开发环境清理：focused skills、manifest、MCP 可见标题、capability registry、命令路由、doctor/eval/smoke 合同统一使用公安经侦和数据研判口径，例如特定双方资金往来核验、资金事实快查、数据质量与口径复核、专项资金核算、异常资金线索研判、证据图表附件、报告事实结论复核、研判材料交付复核和资金流向图谱；同时清理桌面验收遗留 Electron user-data 缓存，保持插件源码目录无本地开发垃圾。

0.15.128 已被 0.15.129 取代；该版本在 0.15.127 基础上完成最终发布身份提升：`FinalInvestigationAnswer` 补证动作纳入调取对象、材料、期间/字段范围、对应证据缺口、优先级、证据边界和禁止升级边界；validation failure、SQL diagnose 和 delivery-qc 类问题进入内部 `audit_events`，普通案件用户仍只看到经侦材料语言、证据边界和下一步补证动作；插件继续只协调和验收可用 Analytix/Codex 交付面，不实现独立 PDF 渲染器、OCR 流水线、资产登记接口或外部证据系统。

0.15.127 已被 0.15.128 取代；该版本在 0.15.125 基础上闭合最终发布候选版本、Analytix-owned runtime cache remount、focused owner 标准化和成熟插件吸收矩阵：workspace manifest、MCP server、capability registry、runtime cache contract、marketplace/skill marker 和 release audit 统一使用 0.15.127；root/index/funds_investigate 保持 navigator，不替代 Pair Amount、画像、追踪、报告、图表和质检 owner；DuckDB workbench 保持当前案件、只读、清洗/analysis 范围和可回放验证。

0.15.125 已被 0.15.127 取代；该版本在 0.15.124 基础上继续收紧真实桌面端前门审计发现的 raw rollout 旧词反射：即便 UI 可见层已经把确认重复记录改写为正向表述，Pair Amount owner 原始成稿也不得引用、否定或加引号复述旧待核类词。新版把 root agent、embedded root agent、Pair Amount owner 和模型可见事实摘要统一加上确认重复记录零旧词合同：已确认重复记录只写为“已确认重复、已剔除、不作为新增转账”，用户用旧词提问也直接回答差异原因。

0.15.124 已被 0.15.125 取代；该版本在 0.15.123 基础上修正真实桌面端前门复测暴露的模型复述风险：当用户在金额挑战追问中带出旧待核类词时，Pair Amount owner、模型可见事实摘要和 Analytix-owned desktop runtime 可见文本边界共同把已确认重复记录固定为“已确认重复、已剔除、不作为新增转账”，并阻断否定式复述。真实前门验收继续使用当前案件进入模型研判窗口，校验两方金额首问、金额挑战、禁词泄漏、active-case 同步和最终回答质量。

0.15.123 已被 0.15.124 取代；该版本在 0.15.122 基础上修正真实桌面端金额挑战追问暴露的旧词否定式泄漏：即便金额结论已经正确，回答也不得把已确认重复记录写成旧待核话术的否定句。新版把 Pair Amount focused skill、路由 metadata 和模型可见事实摘要同步改成正向表述：已确认重复记录、已剔除、不作为新增转账；真实案库 smoke 继续固定原明细、有效事实、已确认重复记录三档拆分，具体案情金额只保存在本地 evidence，不写入发布包文档。

0.15.122 已被 0.15.123 取代；该版本在 0.15.121 基础上修正真实桌面端多轮验收暴露的禁用词门禁缺口：`desktop-multiturn-acceptance.mjs` 同时读取 `*_FORBIDDEN_MARKERS` 和旧命令里常用的 `*_FORBIDDEN`，`frontdoor-smoke.mjs` 对所有 real tasks 都纳入任务级 forbidden markers，避免禁止语检查静默失效；Pair Amount focused skill 和命令元数据同步收紧，已确认重复记录只写成“已确认重复、已剔除、不作为新增转账”，不再把旧待核话术作为反面例子复述给用户。

0.15.121 已被 0.15.122 取代；该版本在 0.15.120 基础上修正真实 Analytix 桌面端前门日志暴露的 Pair Amount fallback：0.15.120 的专门 Workbench SQL 因内部 `SELECT *` 被 guardrail 拦截时，模型会看到旧 `rank_counterparties` 候选聚合。新版把内部 SQL 改为显式列，并把 confirmed duplicate 金额、笔数、状态和说明带入 `key_facts.pair_amount_review`，真实前门 support 摘录稳定输出原明细、去重后有效事实和已确认重复记录三类口径，且把已确认重复记录写为不作为新增转账。

0.15.120 已被 0.15.121 取代；该版本在 0.15.119 基础上修正真实案件库里的 Pair Amount 已确认重复记录表达：原始 23 行的两方转账明细，经同一持有人跨卡/跨账号重复规则应收敛为 18 个有效转账事实；被扣除的 5 行已由当前库内持有人姓名、证件号、方向、时间、金额、余额、对手、交易号/摘要/类型和状态完全一致确认，不再写成“需复核是否重复”。新版把该规则落到 MCP 事实层、support 摘录、Pair Amount focused skill、命令元数据和真实案库 smoke。

0.15.119 已被 0.15.120 取代；该版本在 0.15.118 基础上修正真实 Analytix 桌面端多轮前门验收脚本的旧回答复用风险：当 `turn/start` 没有返回可用 turn id 且线程已处于 idle 状态时，脚本不得把上一轮报告答案当作下一轮全案研判评分对象。新版在每轮发送前记录 assistant 消息基线，发送后必须等到本轮新增 assistant 回答，再执行 marker、用户可见语言、JSON 和内部词泄漏检查。

0.15.118 已被 0.15.119 取代；该版本在 0.15.117 基础上修正真实 Analytix 桌面端前门验收报告场景暴露的最后一处指令式可见措辞：报告正文不得写 `必须保留` 或 `不要合并或改名`，金额争议点改写为 `需在材料中列明`，不同金额类别改写为 `不得混同`。新版把真实报告泄漏句纳入 user-visible smoke，并要求报告/全案 focused skill 发送前扫描这类合同式短语。

0.15.117 已被 0.15.118 取代；该版本在 0.15.116 基础上修正真实 Analytix 桌面端前门验收暴露的可见语言残点：Pair Amount 逐笔备注不得出现 `需结合用途材料复核`，报告/全案表格不得用 `证据状态` 这类诊断表头，下游续追必须显式写出 `暂不能认定` 的证据边界。新版把这些真实泄漏样例纳入 user-visible smoke，并要求 focused skill 在发送前扫描表头、逐笔备注和补证尾部。

0.15.116 已被 0.15.117 取代；该版本在 0.15.114 基础上修正真实 Analytix 桌面端前门验收暴露的交付尾部缺口：金额挑战和主体画像即使事实、金额、表格、异常特征都正确，也必须默认给出可执行的下一步补证/核查建议，区分当前案件内可复核/导出的材料与需外部补取的回单、开户/控制、余额承接、对手方关系、产品/KYC 和用途材料。

0.15.114 已被 0.15.116 取代；该版本在 0.15.113 基础上完成商业级闭环改造：以 Data Analytics 的 source/validation/delivery 纪律作为资金研判母版，以经侦 focused skill 负责最终成稿，并吸收 IB/PE 的 lead-owner、hero deliverable first、support/audit 隐身和专业判断层。新版修复当前案件同步、Pair Amount 事实控制、资金流向图、主体画像、下游续查、全案研判、报告生成、金额挑战/改范围和 active-case 冷启动/切换/恢复的真实桌面端前门验收问题。

0.15.113 已被 0.15.114 取代；该版本在 0.15.112 基础上建立干净发布基线：清理旧 worktree 元数据、历史 runtime 包缓存和旧 closure 产物，保持 0.15.112 经侦研判行为不变，同时让 release tag、Hub 源树、runtime cache 和本机桌面端启动版本重新对齐到同一条当前版本线。

0.15.112 已被 0.15.113 取代；该版本在 0.15.111 基础上继续修正真实 Analytixagent 追下游复测暴露的过度工具调用问题：Pair Amount 自然问句已经能以 1 次工具输出经侦材料式答案，但收款主体后续去向这类续查题仍可能因为资金流向图谱事实摘录过薄而扩展到 27 次工具调用。新版把 fund-flow graph 的模型可见材料升级为一次性中文事实包，直接包含付款主体 -> 收款主体上游金额/笔数/集中日、收款主体后续出账 Top 表、可解释交易边、范围统计、不能认定事项和补证方向；同一真实续查任务降为 2 次工具调用、4,007 字符工具结果，避免模型继续逐个 pair/sql/rank 拼材料。

0.15.111 已被 0.15.112 取代；该版本在 0.15.110 基础上继续修正真实 Analytix 桌面 UI 复测暴露的自然问句成稿问题：Pair Amount 的完整期间、8 个付款账户、6 个收款账户、18 笔、25,885,013 元、逐笔构成和下游承接已经能正确进入最终回答，但普通 `A 转给 B 多少钱` 答案仍可能把 `对手方排行交叉差异` 作为金额核验表一行暴露给经侦用户。新版把排行/明细差异收回到内部复核或短证明边界，除非用户追问金额差异，否则自然答案围绕完整明细结论、集中交易异常、集中交易以外明细意义、逐笔表和已见承接/分流展开。

0.15.110 已被 0.15.111 取代；该版本在 0.15.109 基础上继续修正真实 Analytixagent 线程复核暴露的自然问句成稿问题：Pair Amount 的事实已经能使用完整期间、8 个付款账户、6 个收款账户、18 笔和有效金额，但自然回答仍可能退回多标题金额审计清单，并把 `下一步取证` 写得过重。新版把普通 `A 转给 B 多少钱` 的可见形态收紧为经侦小研判叙事、金额核验表、20 笔以内完整逐笔表和证明力固定短尾，阻断七段标题骨架和用建议替代当前案件研判。

0.15.109 已被 0.15.110 取代；该版本在 0.15.108 基础上继续修正真实 Analytix 桌面 UI 多轮追问暴露的经侦成稿问题：首问 Pair Amount 已能纠正完整期间、8 个付款账户、6 个收款账户、18 笔、25,885,013 元和下游精确承接事实，但追问“继续追下游资金去向”时仍可能复述 `本窗口` 这类 support-layer 词，且把已见 62 笔出账压成较重的下一步取证清单。新版对下游追踪 support 标签做办案语言归一化，并要求 fund-tracing 先解释当前可见流水范围内的理财/基金、第三人、现金/支付通道和断点，再把回单、余额连续性、产品持仓/赎回、KYC/柜面材料作为证明力固定或继续穿透尾部。

0.15.108 已被 0.15.109 取代；该版本在 0.15.107 基础上继续修正真实 Analytix 桌面 UI 自然问题复测暴露的经侦成稿问题：回答已经纠正完整期间、8 个付款账户、6 个收款账户、18 笔、25,885,013 元和 2025 年 2100 万集中交易，但仍可能把已见承接/分流压缩成约数和泛泛证明动作。新版要求 Pair Amount 对当前案内已见下游金额、笔数、对象、账户、产品和日期范围使用精确数，先写为已见案件事实并解释侦查意义，再把回单、持仓/赎回、账户控制、KYC/用途材料作为外部证明力固定边界。

0.15.107 已被 0.15.108 取代；该版本在 0.15.106 基础上继续修正真实 Analytix 桌面 UI 自然问题复测暴露的经侦深度问题：回答已经不再走七段标题清单，也能使用完整期间、账户数量、有效笔数、有效金额、重复边界和已见承接事实，但仍可能压缩成“结论 + 账户列表 + 明细表 + 一句异常”。新版把 Pair Amount focused agent prompt 进一步收紧：自然 `A 转给 B 多少钱` 必须另写两段转账结构研判，分别解释集中交易以外明细的时间跨度、账号变化、金额层级、备注/类型线索，以及重点集中交易的占比、账户/时间集中和已见承接异常意义；20 笔以内完整逐笔表只是证据锚点，不能替代研判。

0.15.106 已被 0.15.107 取代；该版本在 0.15.105 基础上继续修正真实 Analytix 桌面 UI 自然问题复测暴露的经侦表达问题：回答已经能使用完整期间、账户数量、有效笔数、有效金额、重复边界和已见承接事实，但仍可能读起来像标题驱动的金额审计清单。新版把 Pair Amount focused agent prompt 收紧为自然 `A 转给 B 多少钱` 默认先写紧凑经侦小研判：当前流水证明了什么、账户和时间如何演变、重点集中交易与集中交易以外明细分别意味着什么、案内已见承接/分流到哪里；20 笔以内完整逐笔表作为证据锚点，不再让 `统计期间/金额核验/异常特征/下一步取证` 七段式骨架替代研判。

0.15.105 已被 0.15.106 取代；该版本在 0.15.104 基础上继续修正真实 Analytix 桌面 UI 自然问题复测暴露的经侦阅读问题：回答已经能使用完整有效金额、完整统计期间和逐笔明细，但首段仍可能没有把“付款主体相关付款账户多少个、收款主体相关收款账户多少个”交代清楚，办案人员会看不懂所谓完整两方范围。新版在 Pair Amount support envelope 增加 `opening_sentence_must_cover`，并把 focused agent prompt 收紧为未限定日期/账号时首句必须写“某年某月某日至某年某月某日，A相关付款账户N个向B相关收款账户N个转账N笔、合计N元”；重点集中交易只作异常集中和承接分流线索，不能替代完整两方金额。

0.15.104 已被 0.15.105 取代；该版本在 0.15.103 基础上继续修正真实 Analytix 桌面 UI 复测暴露的用户可见污染：最终 Pair Amount 答案已经能围绕完整期间、金额、账户和承接分流形成经侦小研判，但进度栏仍可能回退显示 `Investigate pair amount` 或本地 SKILL.md 读取路径。新版在 Analytix-owned embedded UI 中为资金插件 MCP 工具增加中文办案动作标签，并对 runtime skill 读取命令做可见脱敏；Pair Amount 合同 smoke 同时检查 MCP 元数据和桌面 UI bundle，确保办案用户看到的是“正在核验两方转账/特定双方资金往来核验”，不是函数名或文件系统。

0.15.103 已被 0.15.104 取代；该版本在 0.15.102 基础上修正真实桌面 UI 复测暴露的工具进度污染和自然问句深度问题：MCP 工具面现在为全部工具提供中文 `title`、`annotations.title` 和调用中/完成元数据，避免桌面端回退显示 `Investigate pair amount` 这类英文函数名；Pair Amount focused agent prompt 继续保持在 1024 bytes 内，并要求自然 `A 转给 B 多少钱` 先形成 2-3 段连贯经侦小研判，再用金额表和逐笔表固定依据，尾部只写证明力固定和外部补证边界。

0.15.102 已被 0.15.103 取代；该版本在 0.15.101 基础上修正真实桌面启动暴露的 focused agent prompt 超长问题：`pair-amount-investigation` 的 `default_prompt` 超过 runtime loader 1024 字节限制时会被忽略，导致“已安装新版但行为可能仍像旧版”。新版把 Pair Amount focused agent prompt 压缩到限制内，同时把完整金额控制、小表全量逐笔、`investigative_row_reading`、报告式经侦研判和禁止把已在案分析写成下一步计划等关键合同保留下来。

0.15.101 已被 0.15.102 取代；该版本在 0.15.100 基础上继续修正自然 Pair Amount 首答的“正确但像栏目清单”问题：经侦视角复核显示，答案即使已经纠正完整金额、笔数、期间、账户和逐笔明细，也可能因过度依赖 `统计期间/异常特征/下一步取证` 等标题而读起来像填表。新版把 Pair Amount 默认交付改成紧凑经侦小研判材料：先用连贯段落回答用户这组资金到底怎么发生、资金关系如何演变、当前流水已经看到什么承接/分流，再用金额表和逐笔表固定依据，最后只把外部补证和证明力固定作为尾部。标题只能辅助阅读，不能替代研判。

0.15.100 已被 0.15.101 取代；该版本在 0.15.99 基础上继续修正自然 Pair Amount 首答的“正确但浅”问题：经侦视角复核显示，答案即使已经使用完整明细金额、笔数和期间，也可能仍停留在固定栏目、基础汇总和泛泛取证建议。新版把运行时返回的逐笔交易进一步整理为 `investigative_row_reading` 支撑事实：时间顺序、付款/收款账号分布、重点集中交易、集中交易以外明细、摘要/交易类型线索桶、低额/终端行边界都进入默认分析脊梁。成稿必须先把这些当前案内事实解释为关系持续性、账户变化、用途/控制线索、重复差异和承接/分流意义，再区分已在案可形成材料与需外部补取证明材料。

0.15.99 已被 0.15.100 取代；该版本在 0.15.98 基础上继续收紧自然 Pair Amount 首答：经侦视角复核显示，回答即使已经更接近正确金额、期间和账户范围，仍可能被排行交叉校验、重点集中交易或旧材料牵引，漏掉当前案内完整明细里的早期、小额、终端、批处理、还款、汇款或集中交易以外交易。新版明确“完整明细聚合”是未限定 `A 转给 B 多少钱` 问句的控制事实；排行/重点集中交易/旧材料只能解释差异，不能静默改写总额、笔数、首末时间或逐笔构成。成稿必须把这些旧期/低额/集中交易以外行作为关系持续性、账户变化、用途/控制线索或 rank/detail 差异来解释，不能折叠成残差桶或泛泛下一步。

0.15.98 已被 0.15.99 取代；该版本在 0.15.97 基础上继续收紧 Pair Amount 成稿层：真实复测显示金额、期间和账户范围已经更接近正确，但可见回答仍可能停留在基础会计汇总，一行重点集中交易、一行集中交易以外汇总，再配泛泛取证建议。新版把 20 笔以内返回明细视为当前答案证据表，而不是附件导出提示；成稿必须像办案人员读流水一样解释账户变化、备注/类型、集中交易、集中交易以外明细、重复差异和已见承接/分流，再提出外部证明材料。

0.15.97 已被 0.15.98 取代；该版本在 0.15.96 基础上修正经侦视角复核暴露的 Pair Amount 成稿浅层化问题：`investigate_pair_amount` 已返回完整首末时间、金额/笔数、付款/收款账户范围和逐笔候选，但用户可见回答仍可能把某个集中交易当成主线，或把集中交易以外明细写成“全期间扣除/零散历史往来/需结合用途材料复核”。新版移除通用首句里的集中交易时间示例，要求自然问题首句使用完整首末交易时间和完整有效金额/笔数；20 笔以内完整小表必须逐笔列明；`全期间扣除该集中链路后`、`零散历史往来`、`需结合用途材料复核`、`导出核对该 N 笔明细` 等会被视为用户可见低质表述。

0.15.96 已被 0.15.97 取代；该版本在 0.15.95 基础上继续收紧自然 Pair Amount 首答：用户问 `A 转给 B 多少钱` 时，插件不再把正确总额、统计期间和固定标题视为足够，而是要求先讲清两方转账事实本身，包括涉及账户数量及账号、首末时间、逐笔构成、重点集中交易、集中交易以外往来、重复差异、已见承接/分流和转账结构研判。新版把 `全区间` 等笼统范围词列为用户可见风险，并强化 delivery QC：已有流水里已经出现的后续出账、理财、现金或第三方去向，必须先作为当前案内事实分析，不能写成“下一步调取后续出账”的模板话。

0.15.95 在 0.15.94 基础上继续修正真实 run.sh 桌面 UI 自然问题复测暴露的 Pair Amount 模板化问题：金额事实已经能返回完整统计期间、付款/收款账户范围和逐笔候选，但自然问句默认答复仍可能只给基础汇总、固定标题和泛泛下一步。新版在 support envelope、pair-amount owner、root agent、command metadata、delivery QC 和用户可见语言 smoke 中加入“转账结构研判”要求：必须解释完整两方往来、重点集中交易、集中交易以外往来、重复差异、已在案承接/分流和外部补证边界；已查到的收款方后续出账/理财/第三方去向不得写成需调取的缺失数据。

0.15.94 已发布但被 0.15.95 取代；该版本在 0.15.93 基础上修正真实 run.sh 桌面 UI 自然问题复测暴露的 Pair Amount 成稿问题：`investigate_pair_amount` 已返回完整小表、付款/收款账户范围和统计期间，但最终回答仍可能折叠为“其余若干笔/多个账户”，并把高占比集中收款后的承接核验缩成次日 24 小时。新版要求 20 笔以内完整返回表必须逐笔展开，不得写折叠小额行；高占比集中交易的收款方承接核验至少覆盖集中转入后 30 天或不设过窄截止日，查到已在案承接/分流时必须写入正文，而不是放进泛泛下一步。

0.15.93 在 0.15.92 基础上修正真实 run.sh 桌面 UI 复测暴露的 Pair Amount 首屏和研判深度问题：模型在事实工具调用前仍可能公告 `pair-amount-investigation` 流程，且对高占比主要集中转账只写泛泛“下一步核查去向”。新版把静默选择规则前移到 skill 发现文本、入口 agent、focused agent 和 contract smoke；对高占比集中交易要求先做一次当前案件内 bounded 承接/分流核验，查到产品、现金、第三方或赎回事实时必须写成 `已见承接/分流`，不能埋进下一步取证。

0.15.92 已发布但被 0.15.93 取代；该版本在 0.15.91 基础上修正 Pair Amount Skill 内残留的“最多 10 行”指令冲突：当当前案件返回的有效两方转账表为 20 笔以内时，技能合同、runtime support envelope 和 delivery QC 均要求最终回答列全返回逐笔明细，不得只列前 10 行、重点日期/账户集合或单一汇总行。

0.15.91 已发布但被 0.15.92 取代；该版本在 0.15.90 基础上修正真实桌面 UI 复测暴露的 support budget fallback 漏洞：虽然 frontdoor card 已给到 20 行以内逐笔候选，但 agent-readable support payload 超预算时仍会降级为 10 行，导致自然问题只列重点集中交易和前 10 笔。新版让 Pair Amount fallback 保留 20 行以内完整有效明细，并加入 support-only 完整表标记，要求最终回答列全返回明细，不得只列前 10 行、重点集中交易或汇总行。

0.15.90 已发布但被 0.15.91 取代；该版本在 0.15.89 基础上继续收紧真实 Analytix 桌面 UI 自然问题暴露的 Pair Amount 模板化问题：当当前案件两方有效转账为 20 笔以内时，frontdoor card 和 agent-readable support envelope 都保留完整逐笔候选，`pair-amount-investigation`、command metadata 和 delivery QC 均要求完整逐笔展开，不能只列 2100 万等重点集中交易或最大 10 行。重点集中交易必须回到异常特征、承接分流和案件意义分析中，不能改写未限定时间的完整两方金额结论。

0.15.89 已发布但被 0.15.90 取代；该版本在 0.15.88 基础上修正真实 Analytix 桌面 UI 自然问题复测暴露的 Pair Amount 交付层缺口：金额事实已能使用完整期间、账户数、逐笔候选和重复风险，但自然回答仍容易落成模板化区块，并把已调取流水中的明细/后续承接写成泛泛“下一步核查”。新版要求 `pair-amount-investigation` 在两方总额答复中区分当前案件已见交易、已见承接/分流和外部补证材料；对重点集中交易允许一次 bounded current-case continuation check，避免把当前数据已有内容误写成缺失。

0.15.88 已发布但被 0.15.89 取代；该版本在 0.15.87 基础上修正真实 Analytix 桌面 UI 自然问题复测暴露的 Pair Amount 工具面和研判深度问题：`investigate_pair_amount` 必须进入默认可见工具面并排在排行/导航支持前；agent-readable support envelope 不再暴露会抢结论的排行金额字段，只保留明细控制结论、排行差异、账户范围、逐笔候选、集中交易占比和集中交易以外金额/笔数。`pair-amount-investigation` 新增非模板化深度门禁，普通 `A 转给 B 多少钱` 必须展开首末时间、付款/收款账户数和账号、完整往来与重点集中转移的关系、重复风险、已在案可继续复核事项和外部补证事项。

0.15.87 已发布但被 0.15.88 取代；该版本在 0.15.86 基础上修正真实 Analytix 桌面 UI 自然问题复测暴露的 Pair Amount 范围控制错误：`investigate_pair_amount` 虽已接管自然问句，但支撑事实把重点主要集中转账提升为普通结论，导致未限定时间的 `A 转给 B 多少钱` 被收缩到单日集中交易。新版恢复完整两方往来明细聚合为控制口径，保留首末时间、完整金额/笔数、账号范围、排行交叉差异和集中交易角色；集中交易只作为异常特征和续查线索，除非用户明确指定该日期/账号/交易集合。

0.15.86 已发布但被 0.15.87 取代；该版本在 0.15.85 基础上修正真实 Analytix 桌面 UI 自然问题复测暴露的 Pair Amount owner 接管不彻底问题：`investigate_pair_amount` 在工具面前置，`funds_investigate` 遇到 A 转 B 金额问题会回落到特定双方资金往来核验支撑路径，compact facts 暴露逐笔交易候选、付款/收款账号和集中交易集合，避免自然问法出现英文 support 卡、聚合行替代逐笔表、`我将使用“特定双方资金往来核验”流程` 等机械过程话术。

0.15.85 已发布但被 0.15.86 取代；该版本在 0.15.84 基础上修正真实 Analytix 桌面 UI 自然问题复测暴露的 Pair Amount 工具路径和范围漂移：新增 `investigate_pair_amount` 一等事实工具，普通 `A 转给 B 多少钱` 首答优先使用当前案件重点日期/收款账户主要集中转账、付款/收款账户数、逐笔候选和重复风险，不再把全期间同名历史合计或全量主体画像当唯一答案；同时阻断 `收款端`、`主体账户集合`、泛泛“调取全量流水”等机械用语。

0.15.84 已发布但被 0.15.85 取代；该版本在 0.15.83 基础上修正真实 Analytix 桌面 UI 自然问题复测暴露的来源漂移和下一步取证漂移：Pair Amount 必须优先使用 Analytix 当前案件语义事实工具或已验证受控 Workbench 结果，不能从本地旧报告、旧图脚本、历史文件或 capability gap 拼出金额；当前案件已经有相关流水时，下一步应拆成“在已调取流水中导出/核对”和“需外部补取材料”，不得泛泛要求“调取双方完整流水”。

0.15.83 已发布但被 0.15.84 取代；该版本在 0.15.82 基础上修正真实 Analytix 桌面 UI 自然问题复测暴露的第二类机械研判用语：`A 转给 B 多少钱` 默认必须把可用 `top_transactions` 转成逐笔 `重点交易表`，不得用 `原始命中记录`、`命中记录` 或 `本轮未展开逐笔明细表` 等支撑层/逃避式话术替代交易明细。

0.15.82 已发布但被 0.15.83 取代；该版本在 0.15.81 基础上修正真实 Analytix 桌面 UI Pair Amount 验收暴露的机械研判用语：普通可见正文和表头不得出现 `本轮命中`、`同事实去重`、`证据状态` 等支撑层口径，应改成 `当前流水可见`、`重复风险复核`、`已有流水支持`、`核验意见` 等办案材料语言，并继续要求主体账户数量、账号、期间、金额/笔数、异常特征、案件意义、暂不能认定和下一步取证。

0.15.81 已发布但被 0.15.82 取代；该版本在 0.15.80 基础上修正真实 Analytix 桌面 UI 验收暴露的图谱附件可见污染：资金流向 PNG/JPG、图例、节点、边标签、附件名和 alt text 都属于用户可见输出，不得出现 `supported`、`needs_review`、`candidate`、`edge_status`、`delivery_state` 等工程状态词；复用旧图前必须检查渲染文字，不合规则重新生成或降级交付合规资金链路表。

0.15.80 已发布但被 0.15.81 取代；该版本在 0.15.79 基础上把真实 Analytix 桌面 UI 验收所需的 `agent-thread-locator.mjs` 正式纳入插件包，并把默认搜索范围限定为 Analytix-owned runtime home；只有显式 `--include-system-codex` 时才只读 fallback 到系统 Codex home。0.15.79 的 Pair Amount 首条可见污染阻断保持不变。

0.15.79 已发布但被 0.15.80 取代；该版本在 0.15.78 基础上修正真实桌面 Pair Amount targeted 验收暴露的第二类首条公告污染：0.15.78 的实质答复已通过金额、笔数、统计期间、重点交易表、异常特征、暂不能认定和下一步要求，但首条可见消息仍出现“我会使用 pair-amount-investigation 流程”。本版把“我会使用/流程/commentary/item-*”也纳入首条可见污染阻断，普通案件任务要么直接给结论，要么只写“正在核验当前案件事实。”。

0.15.78 已发布但被 0.15.79 取代；该版本在 0.15.77 基础上修正真实桌面 Pair Amount targeted 验收暴露的首条技能公告污染：0.15.77 的实质答复已通过金额、笔数、统计期间、重点交易表、异常特征、暂不能认定和下一步要求，但首条可见消息仍出现“我将使用 pair-amount-investigation 技能”。本版明确即使选择 focused workflow，也不得向普通办案用户公告技能名、`我将使用`、`特定双方资金往来核验技能`、`先定位当前案件数据` 或 `统计口径`。

0.15.77 已发布但被 0.15.78 取代；该版本在 0.15.76 基础上修正真实桌面 Pair Amount targeted 验收暴露的回合超时：0.15.76 已清除首条进度污染，但普通“A 转给 B 多少钱”仍可能为寻找更多支持证据反复排行/校验而不收敛。本版新增 Pair Amount 桌面首答收敛合同：两方金额、笔数、期间和候选交易已由控制性证据返回后立即成稿，`rank_counterparties` 最多一次候选支持，普通两方金额不得调用报告校验、图谱、画像或全案分析。

0.15.76 已发布但被 0.15.77 取代；该版本在 0.15.75 基础上修正真实桌面 targeted 验收暴露的首条进度污染：Pair Amount 正文已通过金额、笔数、重点交易表、异常特征、案件意义、暂不能认定和下一步取证要求，但首条可见消息仍出现“我会按当前案件资金研判流程先读取案件数据口径与可用交易明细”。本版把普通案件任务改为默认不发进度；如必须发，只能逐字写“正在核验当前案件事实。”，并把该真实泄漏样例纳入 smoke 和桌面验收禁词。

0.15.75 已发布但被 0.15.76 取代；该版本在 0.15.74 基础上修正真实桌面多轮验收暴露的材料标签和进度话术缺口：两方金额材料必须保留“重点交易表/异常特征/案件意义/暂不能认定/下一步取证”，下游续查必须保留“下游去向”，报告续写含金额时必须有独立“金额核对”小节；同时把“按...口径”“先读取...技能说明”和审计层“资金边”纳入用户可见污染阻断。

0.15.74 已发布但被 0.15.75 取代；该版本在 0.15.73 基础上修正真实桌面九场景多轮验收暴露的交付层缺口：图谱普通回答固定使用“资金流向图/资金链路/交易链路”材料式表达，不展示 Mermaid 源码或“资金边”审计层词；主体画像固定保留“重点对手方”；报告续写含金额时必须有“金额核对”；桌面验收 marker 对中文空白不敏感，避免 `16笔/16 笔` 这类等价表达误判。

0.15.73 已发布但被 0.15.74 取代；该版本在 0.15.72 基础上修正真实桌面 Pair Amount 验收暴露的话术收口问题：金额、笔数和去重事实已正确时，用户可见进度仍不得出现执行器、查询、表结构、语义层等工程过程词；两方金额材料必须显式保留“统计期间”和“案件意义”，确保正确事实进入公安经侦材料口吻。

0.15.72 已发布但被 0.15.73 取代；该版本在 0.15.71 基础上修正真实桌面 Pair Amount 闭环：特定双方资金往来核验有主体账户集合内同事实去重聚合时，由该聚合控制有效金额和笔数，专项明细差异只作为大额样例和复核风险；同时把实现层查询/执行器措辞纳入用户可见泄漏阻断，并补齐真实 Electron 多轮验收脚本。

0.15.71 已发布但被 0.15.72 取代；该版本在 0.15.70 基础上收紧用户可见语言收口：旧 support-card 标题不再被改写成“重点收款账户核验”等近似内部标题，金额相关旧“口径/提示”措辞改为“金额核验情况/补证方向”，并扩展前门与功能合约禁词；同名 replay 任务覆盖历史 golden 旧题，避免测试绿但 prompt 或成功 marker 仍像工具卡。

0.15.70 已发布但被 0.15.71 取代；该版本在 0.15.69 基础上完成商业级结构收口：Pair Amount 由独立研判路线成稿，入口、排行、卡片和专项资金核算只提供结构化支撑事实；新增真实清洗明细导出、SQL-safe schema、Analytix 软件工作流上下文、研判材料交付复核、真实文件读取检查和 Hub 包真实案件固定事实阻断。

0.15.69 已发布但被 0.15.70 取代；该版本把 Pair Amount 升级为 `pair-amount-investigation` lead owner，并新增 Analytix workflow context 与 delivery QC：当前案件同步、清洗明细导出、analysis 索引、图谱/PNG/JPG、附件、报告续写、全文金额核算和多轮继续推进都进入 focused workflow 合同；MCP/card/workbench 的 answer draft 只保留为支撑/审计，不再直接构成普通 agent-readable 成稿主体。

0.15.68 已发布但被 0.15.69 取代；该版本在 0.15.67 基础上补齐健康发布链路和用户可见语言细节：release slice 覆盖报告复核诊断运行时及当前生产 MCP 协议文件；普通卡片、报告复核卡和关系图卡片统一使用“交易级可证实资金边”“报告复核要点”等经侦表达，避免“作答骨架”“第一行保留”“已有数据支持的交易级资金边”等审计层措辞进入用户可见输出。

0.15.67 已发布但被 0.15.68 取代；该版本完成早期用户可见语言与交付质量收口，后续版本已进一步移除机械金额标题，并把一跳金额交给 focused owner 组织为结论先行的经侦材料。

0.15.66 已发布但被 0.15.67 取代；该版本在 0.15.65 基础上补齐真实前门闭环：embedded analytixagent 会把当前桌面 backend URL 注入 MCP runtime，避免 active-case 解析落回旧 packaged localhost；Pair Amount/Amount Challenge 前门优先使用确定性 mini-review 卡，`rank_counterparties` 只保留为 candidate/ranking 边界，最终答复必须保留 raw、effective/dedup、可选本金/手续费、duplicate/unsupported、账户集合、时间集中、重复组和下一步核验边界。

0.15.65 已发布但被 0.15.66 取代；真实前门 P0、active-case source guardrail 和 Pair Amount functional closure 继续作为发布前置。

0.15.65 在 0.15.64 基础上修复真实前门 P0：backend active-case 会从 Analytix 上次打开案件恢复，MCP source resolution 在 active/explicit case 缺失时只返回 Case Source Blocker 和恢复动作，禁止本地目录/DuckDB/历史输出猜案；Pair Amount 前门会把 `rank_counterparties` 降级为 candidate-only，并通过当前案件、只读、清洗/analysis scope 的 targeted Workbench 输出 raw、effective/dedup、可选本金/手续费、duplicate/unsupported、账户集合、时间集中、重复组和下一步核验边界；functional closure 新增 env-driven 真实前门 gate，真实案件 marker 只进入闭环产物，不进入 production resource。

0.15.64 已发布但被 0.15.65 取代；Data Analytics 式 source + validation + delivery release gate 保持为发布前置。

0.15.63 已发布但被 0.15.64 取代；Data Analytics 等价闭环和 Workbench release gate 仍需固化到功能闭环证据。

0.15.63 在 0.15.62 真实 Hub 发布和运行时安装后继续修正前门验证证据漂移：真实 Agent UI replay 已证明 Amount Challenge 会走 rank 候选定位、schema/scope/data-quality、duplicate-family 和多次 targeted SQL 聚合，答案也能分开 4200 万 raw、2100 万核心集中交易去重候选、核心账户和证据缺口；但 runner 的 10-tool 预算仍把必要复核误报为 route violation。本版把显式 source-backed replay 预算升到 16，不改变生产 Pair Amount / Amount Challenge 行为。

0.15.62 已发布但被 0.15.63 取代；post-install Agent UI replay 功能通过，但真实复核用了 14 次 analytix_funds 调用，超过旧 10-tool 预算。

0.15.62 在 0.15.61 真实 Hub 发布和运行时安装后继续修正前门验证证据漂移：Amount Challenge replay 题面没有给出具体争议金额和对象时，模型合理选择先澄清而不调用案件工具，不能证明 Pair Amount P0 场景。本版把 replay 题面改成明确的“示例甲 -> 示例乙：重复放大值与高置信核心集中交易值冲突”争议，保留 10-tool source-backed 预算，确保发布证明覆盖金额纠偏链路；生产 Pair Amount / Amount Challenge 行为不变。

0.15.61 已发布但被 0.15.62 取代；post-install Agent UI replay 显示预算已放宽，但题面过泛导致模型可合理先问澄清、未触发案件工具。

0.15.61 在 0.15.60 真实 Hub 发布和运行时安装后继续修正前门验证证据漂移：Amount Challenge 的真实 source-backed 复核链路可能需要 schema/scope/quality、SQL retry、coverage、duplicate-family 和 current-case 检查。本版把 Amount Challenge replay 的显式预算升到 10，并让这种显式高预算的 source-backed 任务不再套用普通 1-2 tool warning，避免验证口径把模型压回弱来源或 rank-only 最终答案；生产 Pair Amount / Amount Challenge 行为保持 0.15.59/0.15.60 不变。

0.15.60 已发布但被 0.15.61 取代；post-install Agent UI replay 功能通过，但 7-tool 显式预算仍不足以覆盖必要的 retry/coverage/current-case 复核形态。

0.15.60 在 0.15.59 真实 Hub 发布和运行时安装后继续修正前门验证证据漂移：Agent UI replay 已证明 Amount Challenge 功能链路正确，但 golden replay 任务构造时未把 `expected_tool_budget` 带入 live task inventory，仍按 2-tool 普通预算报 route warning。本版保留 0.15.59 的生产 Pair Amount / Amount Challenge 行为，只修正发布 runner 的 golden 任务预算继承，使 source/schema、quality、targeted SQL、coverage 和 duplicate family 复核不再被误判为机械预算越界。

0.15.59 已发布但被 0.15.60 取代；post-install Agent UI replay 功能通过，但 golden task constructor 丢失显式预算导致发布证据仍显示 2-tool route warning。

0.15.59 在 0.15.58 真实 Hub 发布和运行时安装后继续修正前门验证契约 drift：Agent UI replay 已证明 Amount Challenge 不再 rank-only，但仍把 `rank_counterparties` 当作必需工具并按普通 3-tool budget 报 route warning。本版把 `rank_counterparties` 降级为可选候选定位，并给 Amount Challenge replay 明确 source/schema、quality、targeted SQL、duplicate family 和 coverage 复核预算，避免必要的 source-backed 聚合被误判为硬越界；生产事实面和系统 Codex 隔离边界保持不变。

0.15.58 已发布但被 0.15.59 取代；post-install Agent UI replay 功能通过，但预算/必需工具契约仍把必要的 Workbench 复核标成 route warning。

0.15.58 在 0.15.57 真实 Hub 发布和运行时安装后继续修正前门验证契约 drift：Agent UI replay、model eval 和 golden fixture 不再把 Amount Challenge 写成 `rank_counterparties` 单工具路径。`rank_counterparties` 只能作为候选定位；最终金额或 competing amount 质疑必须在语义事实不足时继续进入当前案件、只读、清洗/analysis-scope 的 Controlled Case Workbench targeted 聚合，并交代支持金额、不可支持金额、差异原因、验证状态、证据边界和下一步核验。本版不改变系统 Codex 隔离边界，也不把固定案件答案写入 production resources。

0.15.57 已发布但被 0.15.58 取代；post-install Agent UI replay 继续暴露 Amount Challenge 验证提示中的旧 `rank_counterparties` 单工具硬路由。

0.15.57 在 0.15.56 发布身份基础上继续修正 Pair Amount / Amount Challenge 边界：`rank_counterparties` 只能作为排行和候选证据，不能单独最终定额；金额质疑必须核对 source-of-truth、holder/account scope、counterparty grain、raw/effective/dedup、时间窗、同事实/换卡/重复候选，并在语义工具不足时进入 Controlled Case Workbench 的只读清洗/analysis-scope targeted 聚合。本版还移除 production claim review 中的固定案情增强，泛化 packaged references 里的固定金额样例，并把 release guard 的 direct MCP card coverage 评分标成非模型质量证据，不改变系统 Codex 隔离边界。

0.15.56 已发布但被 0.15.57 取代；Pair Amount 金额质疑、production 固定样例和 release guard 固定 marker 继续暴露 drift。

0.15.56 当前工作树在 0.15.55 完整 focused Agent UI A/B 证据基础上继续修正 focused skill drift：Workbench 口径已恢复，但 `subject_dossier_person` 仍可能通过重复账户钻取和 Workbench fallback 超过 focused tool budget，`graph_visualization_supported_edges` 也可能遗漏可见 `candidate` 标签。本版把 Subject Dossier 首轮路径收敛为 `analyze_holder_full` / `rank_accounts` / `rank_counterparties` 三个语义事实工具，把缺失的联系方式、住址、IP/MAC、单位、法人字段写成证据缺口，并固定 `Subject Dossier / 主体画像`、`账户结构`、`归属边界（direct / candidate）`、`不能写成已确认归属`、`candidate`、`needs_review` 和 `证据缺口` 等可见审计标签，不改变 MCP 事实面或工具安全边界。

0.15.55 已发布但被 0.15.56 取代；完整 focused A/B 继续暴露 Subject Dossier 工具预算漂移和 graph candidate 标签漂移。

0.15.55 在 0.15.54 完整 focused Agent UI A/B 证据基础上继续修正 Controlled Case Workbench drift：报告/claim-review 交付骨架已恢复，但 `case_notebook_controlled_query` 在完整套件里仍可把诊断性来源/交付词汇复述到用户可见答案，并把夜间大额出账的全出账分母错误收窄到有交易时间的行。本版把 Workbench 最终答案改成业务化交付表述，并固定夜间大额出账口径为“全出账分母只过滤 `dc_val='出'` 与 `amount IS NOT NULL`，夜间条件只用于分子”，不改变 MCP 工具安全边界。

0.15.54 已发布但被 0.15.55 取代；完整 focused A/B 继续暴露 Workbench 最终答案可出现用户可见诊断词和分母口径漂移。

0.15.54 在 0.15.53 全量 focused Agent UI A/B 证据基础上继续修正报告/claim-review 交付结构 drift：Workbench targeted A/B 已通过，但 `report_builder_generation` 与 `case_3c72_report_claim_review` 仍可把工具返回的报告/claim 复核骨架改写成泛化备忘录，导致发布审计无法证明 report-builder 和 claim-review 的完成门。本版把报告草稿、来源边界、数据质量、未支持 claim、未支持/不能确认资金流、降级/线索、禁用表述和复核动作固化为用户可见交付标签，不改变案件事实、金额口径或 claim verifier 规则。

0.15.53 已发布但被 0.15.54 取代；真实 Agent UI Workbench targeted A/B 通过后，全量 focused A/B 继续暴露 report-builder / claim-review 最终答案可丢失交付骨架。

0.15.53 在 0.15.52 真实 Agent UI Workbench targeted A/B 证据基础上继续修正 Controlled Case Workbench 发布级 drift：插件已能执行 scope/schema/quality/SQL/notebook 并输出三行交付标签，但最终答案仍可复述英文内部来源标签，且重复调用 `inspect_case_schema` 导致 Workbench 工具预算超限。本版把用户/模型注入面改成“原始来源明细/来源明细”等用户可见表述，并把 Workbench 路线收紧为一次 scope、一次 schema、一次 quality、一次 SQL、一次 notebook，不改变夜间大额出账计算口径。

0.15.52 已发布但被 0.15.53 取代；真实 Agent UI Workbench targeted A/B 证明 Workbench tail、数据质量预检和 notebook 创建已可见，但最终答案仍可泄漏英文内部来源标签并重复 schema 预检。

0.15.52 在 0.15.51 真实 Agent UI Workbench targeted A/B 证据基础上继续修正 Controlled Case Workbench 交付尾注 drift：最终答案仍可能遗漏字面 `验证状态:` / `证据边界:` / `能力缺口:`、跳过 `audit_case_data_quality`，并重复调用 `run_case_sql` 读取同一指标。本版把 Workbench final-answer tail 直接写入 Agent 可见 MCP 结果，并把 `get_scope_coverage` 明确降级为轻量 coverage counter，不改变计算口径。

0.15.51 已发布但被 0.15.52 取代；真实 Agent UI Workbench targeted A/B 证明 Workbench 结果可见，但最终答案仍可遗漏三行交付标签并漏掉数据质量预检。

0.15.51 在 0.15.50 真实 Agent UI Workbench targeted A/B 证据基础上继续修正 Controlled Case Workbench 交付标签 drift：工具输出已经带出验证/边界/缺口三行，但最终答案仍可能把 `验证状态:` 改写为 `SQL 执行`；本版把 Workbench 验证状态移到 Agent 可见首行，并明确 `SQL 执行: 已执行` / `Notebook/artifact: 已创建` 不能替代字面 `验证状态:`，不改变计算口径。

0.15.50 已发布但被 0.15.51 取代；真实 Agent UI Workbench targeted A/B 证明 `get_case_scope_map` 和受控聚合结果已可见，但最终答案仍把 `验证状态:` 改写为 `SQL 执行`。

0.15.50 在 0.15.49 真实 Agent UI Workbench targeted A/B 证据基础上继续修正 Controlled Case Workbench 交付标签 drift：受控 SQL/notebook 已能执行且带出聚合结果，但最终答案仍可能遗漏字面 `验证状态:`，导致 source + validation + delivery 合同失败；本版收紧 case-workbench 的 `get_case_scope_map` 预检、最终 `验证状态:` / `证据边界:` / `能力缺口:` 三行和 Agent 可见 Workbench 证据边界摘要，不改变计算口径。

0.15.49 已发布但被 0.15.50 取代；真实 Agent UI Workbench targeted A/B 证明受控聚合结果已可见，但最终答案仍缺少字面 `验证状态:` 交付标签。

0.15.49 在 0.15.48 完整真实 Agent UI focused A/B 证据基础上继续修正 Controlled Case Workbench 交付信封 drift：受控 SQL/notebook 已能执行，但 Agent 可见 compact payload 没有带出小型聚合结果预览，导致模型把成功执行误写成缺失交付；本版仅对 aggregate 或极小 current-case cleaned/analysis 结果暴露 compact preview，并要求 case-workbench 答复写明 `验证状态`、`能力缺口`、证据边界和产品化沉淀目标，避免重复 SQL。

0.15.48 已发布但被 0.15.49 取代；完整真实 Agent UI focused A/B 证明 graph 修复有效，但 Case Workbench 自定义 notebook 任务仍存在交付信封缺口。

0.15.48 在 0.15.47 真实 Agent UI targeted A/B 证据基础上继续修正 graph-visualization 前门 drift：工具顺序已经稳定为 `get_casegraph` + `build_fund_flow_graph`，但最终答案仍可能把 required `确定性交易边（Mermaid 前置事实表）` section 合并进 `数据事实` 或改写成“确定性资金边”；本版要求 graph focused skill、Agent 可见 compiler facts 和 fund-flow runtime contract 在 Mermaid 前原样保留这个独立小标题。

0.15.47 已发布但被 0.15.48 取代；真实 Agent UI targeted A/B 证明 0.15.47 的工具顺序已正确，但 graph 最终答案仍缺少 standalone `确定性交易边（Mermaid 前置事实表）` section。

0.15.47 在 0.15.46 发布闭环中继续修正 release evidence drift：生产 Hub source 曾停留在旧 `0.15.27` 树导致 live package 带入旧 evidence，且 `frontdoor-smoke` 仍用旧 navigator-only/内部 answer card marker 口径；本版确认 Hub package 不含 evidence/eval/golden/oracle/scripts，并把 direct MCP smoke 改为 navigator + targeted semantic tools。

0.15.46 已发布但被 0.15.47 取代；0.15.46 修复了 broad graph request 的 `get_casegraph` 工具顺序，但 release closure 又发现 Hub source/旧 evidence 与 direct smoke 证据口径 drift。

0.15.46 在 0.15.45 真实 Agent UI targeted A/B 证据基础上继续修正 graph-visualization 前门 drift：图谱答案已经保留 required output sections，但广义当前案件关系图/图谱请求仍可能只调用 `build_fund_flow_graph` 而跳过 `get_casegraph`，导致案件上下文和质量边界不足；本版明确 broad graph/casegraph/visual graph QA 先用 `get_casegraph`，再用 `build_fund_flow_graph` 画 supported 资金边，窄源点/续查路径仍可直接进入 flowgraph。

0.15.45 已发布但被 0.15.46 取代；真实 Agent UI targeted A/B 证明 graph output markers 已齐，但 broad graph request 仍可能缺少 `get_casegraph` 上下文工具。

0.15.45 在 0.15.44 真实 Agent UI targeted A/B 证据基础上继续修正 graph-visualization 前门 drift：图谱答案已经保留 `candidate`，但仍可能把 `证据缺口` 改写为泛化“补证重点”；本版把 `证据缺口` 升级为独立最终回答 section，写入 flowgraph delivery contract、Agent 可见 compiler facts 和 graph focused skill output order。

0.15.44 已发布但被 0.15.45 取代；真实 Agent UI targeted A/B 证明 `candidate` 边界恢复，但 graph-visualization 最终回答仍可能缺少 standalone `证据缺口` section。

0.15.44 在 0.15.43 真实 Agent UI targeted A/B 证据基础上继续修正 graph-visualization 前门 drift：claim-review 已恢复发布线，但图谱答案仍可能把 `candidate` 和 `证据缺口` 压缩成普通“需复核”说明；本版把这两个用户可见边界提升到 flowgraph answer card、answer contract、compiler facts 和 focused skill completion gate，确保 Mermaid 只画 supported edge，同时显式保留候选/缺失/需复核和补证缺口。

0.15.43 已发布但被 0.15.44 取代；deep runtime health 证明 claim-review 风险门恢复，但真实 Agent UI targeted A/B 发现 graph-visualization 最终回答仍可能缺少 visible `candidate` 与 `证据缺口` 边界。

0.15.43 在 0.15.42 deep runtime health 证据基础上继续修正 claim-review 卡片 renderer：保留逐项 claim 表格与 `需纠正`、`未支持 claim`、`正式报告暂不出具`、`复核动作` 等交付标签，同时补回 `报告级 claim 需绑定事实来源`、现金/理财/资产去向无 supported transaction edge 的降级边界，以及 `必需事实已齐` 的 supported-claim 正规化状态。

0.15.42 已发布但被 0.15.43 取代；deep runtime health 证明 claim-review 卡片形态已稳定，但专用 renderer 漏掉了 claim-verifier 风险门需要的现金/资产去向和来源边界信号。

0.15.41 已发布但被 0.15.42 取代；真实 Agent UI targeted A/B 证明 graph-visualization 已过发布线，但 claim-review 前门答案仍可能缺少稳定逐项复核卡形态。

0.15.40 已发布但被 0.15.41 取代；真实 Agent UI targeted A/B 证明工具和 marker 已齐，但图谱答案仍可能把五层压缩成普通说明，达不到发布线。

0.15.39 在 0.15.38 真实 Agent UI targeted A/B 证据基础上继续修正 graph-visualization 前门 drift：资金流向图谱工具和 Agent 可见文本会稳定输出 `supported/candidate/needs_review/partial/missing/needs_evidence/unsupported` 状态汇总，把 supported 交易边和候选/缺失/需复核边界分段交付；没有 endpoint-complete supported 边时明确写 `supported 0`、不绘制 Mermaid，并给出证据缺口和补证方向。该版本已发布但被 0.15.40 取代，因为真实前门图谱回答仍未稳定保留 `数据事实` / `不可判断` 交付层级。

0.15.38 在 0.15.37 真实 Agent UI targeted A/B 证据基础上继续修正 claim-review 与 graph-visualization 前门 drift：既有报告复核必须先走 `validate_report_claims`，把报告自述的生成失败或追踪未完成作为待复核 claim/source status，而不是重跑全案；验证卡已覆盖纠正口径、降级边界和补证动作时本轮停止。资金流向图谱 Mermaid 只允许端点完整的 supported transaction edge；缺对手方、unknown、未匹配、同名待复核、candidate、partial、needs_review 或聚合端点只能进入边界/补证表，不能画成确定箭头。该版本已发布但被 0.15.39 取代，因为真实前门图谱回答仍可能丢失状态汇总和证据缺口骨架。

0.15.37 在 0.15.36 真实 Agent UI targeted A/B 证据基础上修正 claim-review 前门 drift：`/analytix review` 明确挂到 claim-review，既有报告复核必须以 `既有报告 claim 复核事实卡` 和逐 claim pass/block/downgrade/needs data 形态交付，验证卡已覆盖纠正口径、降级边界和补证动作时本轮停止，不再追加 schema、SQL、排行、Lab 或全案分析。

0.15.36 在 0.15.35 真实 Agent UI targeted A/B 证据基础上修正图谱交付顺序 drift：graph-visualization 和 fund-flow runtime 都要求先写来源/清洗口径，再列 `确定性交易边（Mermaid 前置事实表）`，最后才用 Mermaid 画 supported edge；候选、缺失、partial、needs_review、聚合对手和未匹配端点只保留为证据缺口、补证或续查对象。

0.15.35 在 0.15.34 真实 Agent UI release audit 基础上修正发布 blocker：claim-review 对用户提交的既有报告文本先走 `validate_report_claims`，并在当前案件内对明确报告金额/路径 claim 做只读、聚合、可复核的确定性纠偏，避免把 fetch failed、raw 重复放大金额或 unsupported path 写成报告结论；graph-visualization 要求 Mermaid 前先给出 `确定性交易边` 事实边界，使图谱输出保留 supported edge、candidate link 和缺口分层。

0.15.34 当前工作树在 0.15.33 真实 Agent UI A/B 证据基础上修正发布 blocker：MCP 运行时对 retryable internal/fetch/lock 类瞬态失败做短重试，避免长前门会话把临时服务抖动写成缺事实；`get_evidence_pack` schema 移除后端不支持的 `entity_ids`，visual-evidence 证据包失败时只输出交付 blocker 和缺口，不再用 0 值占位表冒充事实；case-workbench 夜间大额出账模板固定使用当前清洗/分析索引真实字段 `txn_ts`、`dc_val`、`amount`，claim-review 对明确报告金额/路径 claim 在 validator 证据不足时允许一次 claim-scoped 受控 SQL 复核。0.15.33 在 0.15.32 基础上修正真实 Agent UI A/B 暴露的商业发布偏差：默认语义工具箱显式覆盖 `inspect_case_schema`、`run_case_sql`、`create_case_notebook`，使 Controlled Case Workbench 不再被工具面遮住；focused skills 收紧账户/主体画像、case-context、graph-visualization、claim-review 和 workbench 的首轮 workflow 与交付卡；eval-only claim-review 题面改为显式给出待复核报告 claims，避免隐藏旧答案；发布评分把纯工具预算超限保留为 warning/evidence review，而不把 Data Analytics 允许的补证式分析误封成硬失败。0.15.32 在 0.15.23 基础上把顶级插件化总纲、命令路由、能力矩阵、用户可见 product wording、生产 MCP resource 过滤和发布切片归属重新收紧，并补齐 Data Analytics 式视觉证据交付面和 Controlled Case Workbench：`top-pluginization-plan.md` 继续作为商业化插件系统蓝本，`command-router.md` / `command-metadata.json` 保留 `/analytix` 稳定命名空间与 registry 命令能力族，`visual-evidence` 负责证据表格、Top20 图表、看板卡和附件工作包，`case-workbench` 负责语义工具不足时的受控 SQL/notebook/custom 口径兜底，production `capability-registry` resource 不暴露 eval-only coverage 字段，doctor/release guard 会证明公开文案不携带内部过程措辞、root/skill references 同步、发布切片只包含 Analytix 涉案资金研判插件相关文件；release audit 的质量证据门禁按 12 个 focused Agent UI A/B 产品面核验 `plugin_disabled` 与 `full_plugin`，避免把 38 题 eval inventory 误当作发布前门强制矩阵。0.15.23 在 0.15.22 基础上收紧 Capability Registry 单源化：新增派生 metadata snapshot 合约，doctor/release guard 会证明 command metadata、工具、eval task、passive task 和报告复核 task 都能从同一 registry 派生，并同时校验普通题优先语义工具、报告题进入 claim 复核、passive 非资金题 0 次工具的预算边界；runtime 事实面和 casegraph 组件 evidence pack 行为保持 0.15.22 不变。

0.15.22 在 0.15.21 基础上把 casegraph 最小路线图推进为组件级 evidence pack：主体、账户、户名、对手方、交易边、同事实族、规则命中、理财/资产端/现金断点、缺失回单、evidence pack、claim support 都会生成内部 `evidence_pack`、`claim_support` 和非 supported 组件的 `evidence_boundaries`，并携带 source refs；lead / needs-evidence 组件只能写为续查动作或证据缺口，不能升格为事实。

0.15.21 在 0.15.20 基础上收紧新增 Investigation Lab hard case：把 production `hypothesis_probe` 返回的双空对手方确定性统计写入 Answer Card、Evidence Ledger fact、hypothesis_queue 和现金断点边界，要求最终答案保留 `investigation_intent`、`hypothesis_queue`、`hypothesis_status`、`next_queries`、`stop_conditions` 状态字段，同时仍隐藏内部 query/audit/artifact 编号。0.15.20 在 0.15.19 基础上把 golden 扩展到 21 题/3 案：新增 e7e3 监察委专案 Investigation Lab hard case，专门压测现金断点、双空对手方、理财/资产端线索和候选账户归属假设队列；capability registry 和 scorer 同步要求 `hypothesis_status`、降级原因、`next_queries`、`stop_conditions` 和 supported-edge 边界，避免开放式研判把线索写成已查明事实。0.15.19 在 0.15.18 基础上只改 Agent UI A/B 发布证据基础设施：新增 `--baseline-timeout-ms`，让 `plugin_disabled`/`skill_only` 基线可与插件模式分开设置超时，并把 `timeout_ms`、`baseline_timeout_ms` 和单条 `turn_timeout_ms` 写入质量证据，避免无插件慢跑拖垮 20x5 release-quality 矩阵；生产事实面、MCP tools、Evidence Ledger、Context Compiler、Claim Verifier 和 Hub 安装行为保持 0.15.18 不变。0.15.18 在 0.15.17 基础上收紧非案件 passive 行为：普通改写、公式、脱敏和会议事项必须保留用户给定的数字、日期、时间、符号和请求格式，同时不触发 `analytix_funds`；Agent UI passive copy-edit 评分把 `上午10时` 纳入“上午十点/上午10:00”等价时间，修正非资金普通改写的过严 marker。0.15.17 在 0.15.16 基础上把已验证插件切片回灌到当前 Analytix mainline，避开既有 `analytix-fund-analysis-v0.15.16` tag 挪动风险；runtime 行为保持 0.15.16 不变，但要求以新 tag 重新生成 mainline release identity、Hub publish/install、runtime remount、B0、health 和 Agent UI A/B 证据。0.15.16 在 0.15.15 基础上把 `top-pluginization-plan.md` 收口为更完整的系统总纲和后续 Agent 规程：补齐端到端运行链路、领域对象模型、交付物生命周期和能力全集，同时保留硬验收口径，要求 `full_plugin` 必须证明强于无插件 Codex 与低能力基线；root references 与 skill embedded references 保持同步，doctor/runtime cache/Hub package 继续检查同一文本。0.15.15 在 0.15.14 基础上修正真实 Agent UI 安装路径的 backend URL 注入：`.mcp.json` 不再硬编码 `env.ANALYTIX_API_BASE_URL`，只保留 `env_vars` allowlist，使 app-server/runtime env 能把当前后端地址注入 MCP child；doctor 和 release guard 会阻止该字段再次被写死。0.15.14 在 0.15.13 基础上保留 Agent UI app-server `cwd`、`PWD`、`INIT_CWD` 隔离和 `client_process_cwd` 证据记录，但将 `PWD` 环境覆盖改为 computed key，避免发布包文本审计把安全的 cwd override 误判为 secret-like assignment；runtime 事实面、Hub 包机制和 0.15.13 资源暴露链路保持不变。0.15.13 在 0.15.12 基础上继续加固 Agent UI 发布质量证据：自启 analytix-agent app-server 的进程 `cwd`、`PWD`、`INIT_CWD` 统一钉到 eval cwd，A/B 产物记录 `client_process_cwd`，release guard 检查 cwd/PWD 隔离；同时把后续 Agent 施工规程、外部借鉴机制映射、A/B 证明设计和资金链路穿透验收标准写入顶级插件化总方案。runtime 事实面、Hub 包机制和 0.15.12 资源暴露链路保持不变。

0.15.12 在 0.15.11 基础上加固 Agent UI 发布质量证据：release-quality A/B 默认 read-only、启动前要求干净 worktree、运行中若出现除指定 output artifact 外的任何 git 变更则把本轮证据标记为 invalid；同时只自动批准明确只读的 `cat`/`sed` 命令，拒绝文件变更和权限升级，并让自启 analytix-agent app-server 使用 eval cwd 而非 release repo cwd，降低质量跑污染 release 文件的风险；还把插件包边界、外部借鉴取舍、后续 Codex 施工规程写入顶级插件化总方案。runtime 事实面、Hub 包机制和 0.15.11 资源暴露链路保持不变。

0.15.11 在 0.15.10 基础上把顶级插件化总方案扩展为完整的产品、插件架构、程序施工、Hub lifecycle、Agent 执行和发布门禁手册，使后续 Agent 可从 packaged references 自动读取同一规程并继续推进；runtime 事实面、Hub 包机制和 0.15.10 资源暴露链路保持不变。

0.15.10 在 0.15.9 基础上把顶级插件化总方案纳入 root/skill references、progressive resources、doctor、release guard、release-slice audit 和 runtime-cache contract，使 Evidence Ledger、Context Compiler、Claim Verifier、casegraph/fundgraph、Investigation Lab、Hub lifecycle 和发布质量门禁成为可打包、可发现、可复核的同一规程；runtime 事实面和 0.15.9 claim-support 行为保持不变。

0.15.9 在 0.15.8 基础上补齐 casegraph/fundgraph 证据包与报告门禁：supported 资金边、Top 账户、户名和对手方排名都会写入内部 evidence_pack/claim_support，unsupported Mermaid/法律定性/候选账户归属/现金理财去向 claim 会被 doctor 硬门禁拦截；同时收紧 release-slice quality evidence inventory，旧 A/B 结果不能冒充当前 dirty 补丁发布证据，并把授权后的 Agent 自动发布规程写入 Hub lifecycle。0.15.8 在 0.15.7 基础上修正 Agent UI 发布评分器的 passive copy-edit 时间 marker 等价识别：`上午10点`、`明日上午10点` 与“上午十点”同义命中，避免保留原文时间语义的普通改写答案被误判为缺失事实；runtime 事实面、Evidence Ledger doctor 合约、工具预算和 Hub 生命周期行为保持不变。0.15.7 在 0.15.6 基础上新增 Evidence Ledger 可追溯 doctor 门禁：金额、笔数、账户、户名、对手方、资金边和报告 claim 的 ledger 覆盖必须携带 source refs；有锚点样本必须通过、无锚点样本必须失败，后续发布不得只靠人工目测证据链。0.15.6 在 0.15.5 基础上修正发布评分器的账号脱敏 marker 等价识别：字面 `*`、Excel `REPT("*", ...)`、SQL/Python `REPEAT("*", ...)` 与“星号”同义命中，报告复核中的中文“进/出方向”也与 `directed` 同义命中，避免正确的非资金账号脱敏答案和中文事实边界被低估；runtime 事实面、工具预算、Hub 生命周期规程和报告 claim review 行为保持不变。0.15.5 在 0.15.4 基础上修正发布评分器的证据边界识别：明确写有“未形成 supported transaction edge”“缺少凭证/回单/余额承接”的报告复核文本不再被误判为“已确认资金闭环/最终去向”，普通 CSV 导入建议里的“交易时间/时间字段”也按“日期”同义命中；同时把 Hub 发布、安装、remount、回滚和质量证据复核规程固化到 `references/hub-lifecycle.md`。0.15.4 在 0.15.3 基础上加固真实 Agent UI A/B runner：自启 Analytix-owned `analytix-agent app-server` 默认使用 90 秒 readiness timeout，缺少临时 worktree 二进制时会先给出明确 binary preflight 错误，并把 `app_server_binary_path`、`app_server_ready_timeout_ms` 写入质量证据。0.15.3 在 0.15.2 基础上扩展 passive near-miss golden 覆盖、收紧数据质量 Answer Card 必写边界，并新增只写 `/tmp` 的 Hub 包源预检脚本，确保发布前可复核 package source、marketplace stub、archive SHA256 和 golden isolation，同时不把包源预检、runtime cache sync 或本地 remount 冒充为 Hub publish/install。0.15.2 在 0.15.1 基础上要求报告门禁答案保留可见 `fact_refs/source refs` 边界，同时继续隐藏内部查询、审计、artifact 和 evidence 编号。0.15.1 在 0.15.0 基础上收紧报告级 Answer Card 输出契约：最终报告门禁答案必须保留 claim-review `risk_marker`、可见 `source refs` 边界和“不能写成”归属降级表述，避免模型把候选账户、现金/理财去向、缺失对手方断点和缺少来源锚点的风险类别改写丢失；仍不暴露 `q_*`、`audit_ref`、`artifact_id` 等内部审计编号。0.15.0 候选在 0.14.9 基础上强化报告级 Claim Verifier：候选账户升级为名下账户、现金/理财/资产最终去向、缺失对手方/现金断点闭环、敏感 claim 未绑定 `fact_refs/source_refs` 均会进入 unsupported/boundary/next-review 门禁；Evidence Ledger 内部记录 `claim_id`、`claim_category` 和 `risk_marker`，golden 覆盖扩到 18 题/3 案/4 个报告复核任务。0.14.9 在 0.14.8 基础上只修正发布评分器的资产/理财降级表述 guard：明确写有“而非/不写作最终去向”的报告边界不再被误判为现金去向越界；runtime 事实面、工具预算和 Answer Card 契约保持 0.14.8 行为。0.14.7 起补齐发布身份和任务清单审计：`model-ab-eval --list-tasks` 可直接输出多案多题、passive 非介入、报告题和 registry 派生工具预算；`agent-ui-ab-run.mjs` 会在 A/B 产物写入 manifest/server/HEAD/tag 身份；`release-slice-audit.mjs` 默认暴露 manifest/server/tag/release-notes 身份、未提交发布切片和非插件脏路径，并可用 `--quality-evidence <json>` 只读复核真实 UI A/B 产物是否满足当前候选版本、完整记录数、`full_plugin > skill_mcp` 和 `auto_fail=0`；`--quality-evidence-inventory` 可只读扫描现有 A/B 产物并列出为什么没有当前 HEAD/version 可发布质量证据；`doctor` 会本地检查 `plugin.json`、skills、`.mcp.json`、interface metadata 和资产路径是否符合插件 ingestion 结构；`sync-runtime-cache.mjs --apply` 只能在既有同版本 Hub 安装和 runtime skill mount 上用 `--confirm-local-remount` 显式执行本地 remount；Evidence Ledger 的 `coverage_summary` 会按截断前的全量扫描条目标出关键金额/笔数/账户、户名、对手方、资金边/report claim 条目是否缺少 source refs，避免输出上限后的事实缺口被隐藏；claim review card 还会用内部 `claim_support_index` 把每条已复核/需纠正/未支持 claim 和复核动作挂到 ledger，不暴露给用户可见文本。

0.14.6 在 0.14.5 基础上补齐临时 schema/锁边界和发布切片闭环：DuckDB/schema 临时不可用会作为 retryable evidence boundary 传播到后端契约、case-scope MCP 卡片和 skill references，不再渲染成核心表缺失或 0 值事实；真实 UI A/B 和模型评分的报告题工具预算改为从 capability registry 派生；新增只读 `release-slice-audit.mjs`，用于候选版本、临时隔离 worktree、staging plan 和 readiness plan 审计，确保混合脏工作区也能只纳入插件发布切片。`0.14.0` tag 保留为历史 RC，`0.14.1`/`0.14.2` 为本地候选，`0.14.3`/`0.14.4`/`0.14.5` 为历史发布候选，`0.14.6` 为上一发布候选。

0.14.5 在 0.14.4 基础上补齐主体分析必需事实门禁：`holder_analysis` 的主体范围、账户统计和 Top 账户只允许来自成功的确定性子工具；子工具失败时前门返回 `partial`、`required_facts_present=false` 和 blocking warning，不把缺失事实渲染成 0。

0.14.4 在 0.14.3 基础上把 live `check-health` 的固定事实族断言改为先做 deterministic preflight：选中案件包含该合成事实族时运行强 eval 断言，不包含时仍保留 generic live health，preflight API/runtime 不可用则硬失败；同时为 active-case health 加本地临时锁，避免并发健康检查互相覆盖当前案件，并补齐 release gate 脚本、release notes、thin server、runtime cache 边界说明的 doctor/release guard 覆盖。

0.14.2 在 0.14.0 RC 基础上补齐 live Evidence Ledger 健康门禁、`validate_report_claims` 的可见 `claim_review_card` 包装、后端 claim review 对象到可读文本的归一化、capability registry/command metadata 工具预算漂移检查，并修复真实 UI A/B runner 的工具序列分析初始化问题。

0.14.0 将涉案资金研判插件从“前门事实查询”升级为协议级研判系统：普通事实题一次 `funds_investigate` 后直接作答，报告复核题最多追加一次 `validate_report_claims`，非资金 passive 任务不得调用 `analytix_funds`；`answer_card` 固化 `answer_card_complete`、`recommended_next_action`、`max_additional_tools`、`required_facts_present`、`unsupported_flows_present` 和 `context_compiler` 契约，完整事实卡禁止继续无效 discovery；MCP 结果通过内部 `_meta.analytix_evidence_ledger` 记录金额、笔数、主体、账户、对手方、资金边和报告 claim 的本地证据账本，用户可见文本仍不暴露 `q_xxx/audit_ref/artifact_id`，默认模型上下文只看压缩事实卡，完整 `structuredContent` 仅在 `include_debug=true` 且受控环境变量 `ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD=true` 时开放；`claim_review_card` 输出 verified/corrected/unsupported/boundary/forbidden/next actions，并对报告正文执行金额 claim、Mermaid/箭头链路、法律敏感表述和来源边界缺口的确定性风险扫描；`Investigation Lab` 输出侦查意图、开放式假设队列、证据状态、金额集中、重复/同事实风险、缺失对手、理财/资产线索、降级假设、下一步查询和收口边界。Phase4 eval 增加 task-specific scoring、工具序列/重复调用审计、runtime cache 一致性检查和 release guard；真实 UI A/B harness 会在质量评分前阻断缺失/陈旧 Analytix-owned runtime cache，并把过度调用、绕过前门和 passive 误触发写入 route violation。auth 边界只允许复用 Analytixagent/Codex provider 的模型登录态，插件不读取、修复或迁移 token/provider config，eval/runtime 输出会脱敏 Bearer token、API key、JWT、refresh token 和 session token。golden 从 10 题扩到 15 题，覆盖第三案件、非资金 passive 不触发插件、Mermaid/法律敏感表述报告门禁。历史 RC 产物中 `B0 hard_diff=0`，`B1/B2` 无 auto-fail，完整 10x5 中 `full_plugin` 平均 97.7、auto-fail 0，高于 `skill_mcp` 86.9；后续改动后仍需重跑动态门禁。

0.12 起插件新增强资金分析内核入口；0.13.9 补充 `get_case_scope_map`、`resolve_duplicate_families`、runtime 安装缓存体检、MCP resources、MCP 参数兼容层和紧凑案件入口；0.13.10 收紧 `/analytix rank` 机器路由；0.13.11 将 `get_current_case` 的案件看板计数重命名为 `case_dashboard_stats` 并标注“不是分析覆盖口径”；0.13.12 新增 `funds_investigate`、`get_casegraph`、`build_fund_flow_graph`，并将 MCP 默认文本输出收敛为 `answer_card + key_facts + warnings + evidence_refs + next_actions`；0.13.13 将默认 `tools/list` 收敛为前门和高价值事实工具，避免几十个低层工具同时竞争，调试/兼容时可用 `ANALYTIX_FUNDS_DISCOVERY_MODE=full` 暴露全量工具；0.13.14 进一步把默认 `structuredContent` 也压成低上下文卡片，完整 JSON 改为本地 MCP audit artifact/detail_ref，只有显式 `include_debug=true` 才回传完整结构，避免真实模型在最终报告阶段被大 JSON 拖垮；0.13.15 将默认工具发现进一步收敛到少数前门/图谱/实验室/校验工具，单次输出预算降到 6k 左右，并让 `hypothesis_probe` 容错 `counterparty_name`/`via_holder_name`，避免强模型在长任务里反复翻工具菜单和补 schema；0.13.16 把 Agent 可见协议从“审计编号驱动”改为“事实卡片驱动”：默认文本和 `structuredContent` 删除 `q_xxx/audit_ref/detail_ref/artifact_id/evidence_refs` 等内部编号，`include_debug=true` 在生产默认不展开完整 JSON，同时 `build_fund_flow_graph` 默认返回足够回答“给谁/又给谁”的 `edge_preview`（端点、时间、金额、交易号、分类和边界），避免模型为追下游打开大包；0.13.17 进一步把 `rank_accounts/rank_holders/rank_counterparties` 从默认工具发现中收起，改由 `funds_investigate` 内部按问题路由排名，并在户名分析前门中直接返回 holder_top_accounts/rankings、重复风险和答复契约，减少强模型在相邻工具间试探；0.13.18 修复前门结果被二次压缩后金额归零的问题，普通问答遇到数据质量门时返回“可答复但非报告级”边界，避免模型因 `partial` 状态反复追加工具；0.13.19 将默认工具发现收敛为统一前门 `funds_investigate` 加两个校验器，并把 Agent 可见文本从 JSON 改为“答题卡/关键事实/质量边界/停止规则”，高级图谱、排名、lab 和探针只由前门内部或显式 full discovery 使用；0.13.20 关闭默认 `structuredContent`，避免 analytixagent 把结构化对象优先写成大段 JSON 给模型，生产默认只返回人可读事实卡片，只有受控 debug 才展开结构化对象；0.13.21 在“主体去向 + 续查对象”前门里内联续查对象后续外部去向排名和收束提示，减少强模型为同一问题重复调用前门/图谱；0.13.22 修复 ranking 前门：复合排名问题会同时返回往来、出账、入账、笔数或最大单笔的多口径 Top 卡片，并在输出压缩时优先保留排名事实，避免只剩质量边界；0.13.23 修复质量复核前门：`qa_review` 显式意图不再被“资金穿透风险”等措辞误判为资金去向任务，重复/换卡/补卡/空户名/清洗问题优先进入数据质量与口径复核 lane。`get_current_case` / `get_case_status`、`inspect_case_schema`、`audit_unindexed_sources`、`audit_case_data_quality`、同事实重复/换卡候选族、`get_case_reconciliation`、`get_scope_coverage` 和导入/清洗/户名树摘要被压成 Analytix 版数据图谱/口径图谱。`get_current_case` 默认只返回案件身份、案件看板元数据和导入摘要，只有显式 `include_imports=true` 才返回 compact 导入行；`get_case_scope_map` 默认不拉树明细，子步骤短超时降级，`include_child_details=true` 也只返回 compact child details，避免 Agent 因入口工具过慢或过大而放弃事实链。MCP `resources/list`/`resources/read` 暴露根 skill、命令路由、机器可读 metadata、工具可用性、runtime boundary、Hub lifecycle、反模式、插件借鉴边界、casegraph roadmap 和 capability registry；golden/oracle 只允许存在于 `scripts/eval-fixtures/` 和本地 eval 脚本路径，不进入生产 resources。MCP 参数兼容层只做窄口径归一化。Agent 只有在后端返回覆盖率、一致性校验、未纳入口径来源审计、同事实族去重边界和必要的数据质量体检后，才允许使用“全量”“全部覆盖”等报告措辞；交易明细查询仍只作为样本，不作为模型侧全量统计来源。

0.13.24 将 `qa_review` 升级为低上下文事实体检卡：前门直接返回导入阶段交易去重数量、清洗标记、同一人账户集合同事实候选、账户内自然重复、空户名解析、未登记账户和对手字段缺口等硬指标，避免无插件路线反复 dump schema/手写 SQL 才得到同类结论。

0.13.25 稳定 `casegraph`/`qa_review` 的 schema 与未纳入口径来源审计链路：schema/source_audit 先完成再进入重型质量步骤，并对瞬时后端错误做短重试；即使后端抖动也只返回人可读边界，不把 `unexpected server error` 这类内部错误推给模型或用户。

0.13.26 修复复合排名题的低上下文事实卡：`funds_investigate(intent=ranking)` 现在能在“按往来、入账、出账、笔数、最大单笔分别 Top”这类并列问法中稳定包含笔数口径，并在排名行内显示最大单笔金额，避免模型只能提示“需进一步明细核验”。

0.13.27 修复真实模型漏参容错：当 Agent 只传 `intent=ranking/top_n` 而没有把自然语言问题放入 `question` 时，排名前门默认返回往来、出账、入账、笔数、最大单笔五个常用侦查口径，避免 GPT5.5/DeepSeek 因参数省略而退化成单一往来榜。

0.13.28 修复资金去向高风险口径：`destination` 前门在同一主体账户集合内对对手方排名启用有效 `txn_id` 同事实折叠，避免换卡/补卡/同交易号跨账户重复把“示例甲 -> 示例乙”的核心链路放大为两倍；同时对指定续查对象加深内部 Top 出账窗口并返回“主体 -> 续查对象”去重汇总和最大日期集中，帮助模型解释原明细与高置信去重差异而不是自行猜测。

0.13.29 将同事实折叠键收紧为有效 `txn_id + 时间 + 方向 + 金额 + 余额 + 对手`，避免只按交易号误折叠真实交易；`rank_counterparties` 新增 `counterparty_group_mode`，前门回答“资金转给谁”默认按对手户名聚合，同时保留账号级下钻。合成 eval 固化“示例甲 -> 示例乙”的户名聚合、核心集中交易和重复放大阻断关系，禁止再次把重复放大值输出为事实。

0.13.33 修正真实 UI A/B 评分器的错误锚点：重复放大值只作为“禁止写成事实”的风险提示，不再要求答案额外复述该数值，避免评分规则反向鼓励模型把错误金额写得更像事实。

0.13.32 收紧普通问答的工具预算：短 skill 和真实 UI A/B harness 都明确“普通资金问答/后续去向/Mermaid 图谱题只应一次 `funds_investigate` 后作答，不调用 `validate_report_claims`，也不重复前门”；`validate_report_claims` 只用于正式报告或报告级段落，避免完整插件模式把 claim review 误用成普通问答的重复调用链。

0.13.31 补强“示例甲 -> 示例乙”时间窗口一跳核验的核心账号输出：事实卡会显式给出付款核心账号与收款核心账号，避免 Agent 在核心账户问题中只写金额不写账号；同时修复真实 UI A/B harness 的 `interrupted` 终止态和超时自动中断，防止无插件基线后台卡住污染后续组；评分器增加金额逗号/空格归一化，避免格式差异造成漏答误判。

0.13.30 增加四组真实模型 A/B 基线和 eval-only 黄金答案集：`plugin_disabled`、`skill_only`、`skill_mcp`、`full_plugin`，把标准事实、禁止误写金额、允许不确定表述固化在 `scripts/eval-fixtures/golden-answer-set.json`；`model-ab-eval`/`agent-ui-ab-run` 会记录工具调用、命令执行和 route violation，禁止“无插件未实际读库却给金额结论”。MCP resources 增加 `plugin-benchmark.md`，记录 OpenAI Skills/Plugins、CodeGraph、Impeccable、oh-my-codex 等可借鉴边界；`destination` 前门补充 2025-01-01 起时间窗口一跳核验，避免模型为补数字重复调用。

普通插件 MCP 继续固定 disabled；它的受控协议 `tools/list` 仅含两个不可由普通
Agent 执行的 host-capture schema，不构成 provider-visible 工具面；
`ANALYTIX_FUNDS_DISCOVERY_MODE=full`、
`ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS=1` 等历史调试变量不得恢复 dormant 工具。
Go 宿主的 reserved catalog 仅可动态广告 `analyze_account_flows`，且必须在每次
受保护数据访问前重验同案、同轮、同快照 authority；语义事实工具箱与 Controlled
Case Workbench 仍是后续独立范围。

`run_full_case_analysis` 内部也会强制执行 `plan_case_analysis`、`audit_case_data_quality` 和 `run_investigation_lab`，并把数据质量边界、`mandatory_review_cards` 写入报告草稿和 claim validation facts。全案报告会把已生成且 case/scope 匹配的 `plan_context` 传给 lab，避免重复规划扫描；standalone `/analytix lab` 仍会自行规划。lab 会返回 `coverage_context` 区分计划覆盖率和本次探针日期窗口。这样即使 Agent 只调用全案报告工具，也不会跳过导入清洗/换卡重复体检或开放式深挖入口。

## 健康检查

插件包内置两个检查入口：

- `scripts/doctor.mjs`：静态诊断插件文件、MCP 工具面、机器可读路由、`command-router.md` / `command-metadata.json` 覆盖一致性、registry-declared functional closure contract、fixture 隔离、渐进披露和 Codex 隔离边界。
- `scripts/check-health.mjs`：只读运行健康检查，覆盖 manifest、MCP cwd、MCP tools/list、resources/list、prompts/list、后端 health 和可选案件 smoke test。
- `scripts/functional-eval.mjs`：按当前插件版本验证 Data Analytics 式 source + validation + delivery 功能闭环；旧 A/B / scorer 脚本只保留为诊断背景，不作为发布质量证据。
- `scripts/skill-clause-audit.mjs`：逐 clause 审核 Analytix skills，对照 Data Analytics / Investment Banking / Public Equity Investing 的 router、source guardrail、owner、evidence、delivery 和 completion-gate 做法。
- `scripts/mcp-return-oracle.mjs`：不跑模型，直接用只读 DuckDB oracle 校验 Workbench MCP 返回的 schema、计数、聚合、profile、preview、diagnose、EXPLAIN、安全阻断、Top-N 可见文本、destination 自转过滤后的 Top-N 完整性，以及图谱/证据基础表是否正确。
- `scripts/frontdoor-p0-oracle.mjs`：读取真实 analytix 前门线程或 SSE replay 证据，逐题校验模型是否走正确 owner、工具、Top-N、Pair Amount source-of-truth、Workbench ladder、用户可见语言和报告/附件检查；没有真实线程证据时必须失败，不能用 direct MCP 冒充 L3。

```bash
node plugins/analytix-fund-analysis/scripts/doctor.mjs
node plugins/analytix-fund-analysis/scripts/check-health.mjs
node plugins/analytix-fund-analysis/scripts/check-health.mjs --case-id <case_id>
node plugins/analytix-fund-analysis/scripts/check-health.mjs --case-id <case_id> --deep
ANALYTIX_CASE_PROJECT_ROOT=/Users/sun/Projects/<case-project> node plugins/analytix-fund-analysis/scripts/check-health.mjs --case-id <case_id> --deep --report-dry-run
node plugins/analytix-fund-analysis/scripts/eval-coverage-contract.mjs --json
node plugins/analytix-fund-analysis/scripts/functional-eval.mjs --list-tasks --json
ANALYTIX_FUNDS_ORACLE_CASE_DB=<case.duckdb> node plugins/analytix-fund-analysis/scripts/functional-eval.mjs --skip-backend --json
node plugins/analytix-fund-analysis/scripts/skill-clause-audit.mjs --json --fail-on-hard-gaps
node plugins/analytix-fund-analysis/scripts/mcp-return-oracle.mjs --json --case-project-root /Users/sun/Projects/<case-project> --case-db <case.duckdb> --require-case-db --fail-on-gaps
node plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs --json --case-project-root /Users/sun/Projects/<case-project> --case-id <case_id> --require-installed-version <version> --fail-on-gaps
node plugins/analytix-fund-analysis/scripts/mcp-surface-snapshot.mjs --output output/analytix-fund-analysis/evidence/mcp-surface-snapshot.json --fail-on-violation --json
node plugins/analytix-fund-analysis/scripts/mcp-surface-snapshot.mjs --self-test-contract --json
node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --self-test-mcp-surface-evidence --json
node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --candidate-version <version> --functional-closure-evidence output/analytix-fund-analysis/functional-closure/<timestamp> --mcp-surface-evidence output/analytix-fund-analysis/evidence/mcp-surface-snapshot.json --readiness-plan --json
```

MCP surface V2 快照绑定普通插件 manifest、`.mcp.json`、生产入口闭包契约和六个
生产文件的完整 SHA-256，并保存原始 JSON-RPC 观察；它只证明两个固定
host-capture schema 及其 JavaScript 隔离契约，不证明 Go host-owned dynamic
catalog、案件事实执行或发布授权。同版本旧快照、超出固定协议目录的普通 MCP
工具面、额外资源、prompt 参数、边界文本漂移、重复 JSON key、符号链接或自报
`publication_ready=true` 均会被拒绝。
`--fail-on-publish-blockers` 会隐式执行完整 readiness 检查，且缺少 functional
closure evidence 时保持 fail-closed。

默认检查不会生成报告文件。`doctor` 还会检查当前 git diff 是否触碰 `analytixagent/`、`CodexDesktop` 或 `codex/` 边界；出现这类变更必须单独确认，不能作为资金研判插件改造混入。健康脚本启动 MCP 子进程时只传递 `.mcp.json` 允许的 analytix 变量和少量系统路径变量，不继承整份宿主环境。当前案件来源以案件项目工作区 `.analytix/case-project.json` 为准；显式 `case_id` 只能匹配该绑定，不能用后端全局 active-case 或本地目录搜索兜底。`--deep` 会调用 `scan_case_risks`，耗时可能达到数十秒；`--report-dry-run` 只应在发布前专项复核时使用，并会强制 `write_report=false` 且不得写盘。兼容参数 `--report-write-smoke` 现在只验证 `write_report=true` 被 PublicationReceipt gate 固定阻断、无路径和无文件写入；它不再执行真实报告写入。

MCP 默认通过当前对话工作区解析案件项目：`workspace -> .analytix/case-project.json -> analytix data-analysis case -> case.duckdb`。如果案件项目绑定不存在，应从 analytix 案件项目内打开 Agent；工具参数中的显式 `case_id` 必须与该绑定一致。插件不使用 Analytix_new、后端全局 active-case、本地环境变量或历史输出作为陈旧案件兜底，避免误读其他案件项目。

### 发布前真实功能闭环验收

发布前验收不再以旧式 A/B、平均分、`full_plugin` 分数或一次模型成稿为主线。正式评估候选版本时，先证明当前 Analytix 内嵌 Agent + 当前插件包 + 当前 runtime cache + 当前案件数据能完成 Data Analytics 式 `source + validation + delivery` 闭环：Pair Amount、Object Dossier、Full Case、Trace/Lab、Workbench、Visual/Report 和 Passive 场景都有 source envelope、validation state、delivery state、用户可见输出和失败分类。Workbench 和 DuckDB 正确性以 `mcp-return-oracle.mjs` 的确定性返回校验为准；skills 是否正确推动模型，以 `skill-clause-audit.mjs` 的逐段契约审计为准。旧 `model-ab-eval.mjs` / `agent-ui-ab-run.mjs` 可作为诊断工具使用，但不得替代 functional closure evidence。

需要由 runner 自启 Analytix-owned `analytix-agent app-server` 时，使用 `--spawn-app-server`，默认 app-server health readiness 为 90 秒；慢机器或冷启动可显式加 `--app-server-ready-timeout-ms <ms>`，避免 30 秒 DirectClient 默认值造成发布证据假阴性。`analytixagent/src/<platform>/analytix-agent` 是内部 eval/app-server 二进制路径，不是产品品牌或第二 runtime；临时 release worktree 若没有该路径，必须显式传 `--app-server-binary <path>` 或设置 `ANALYTIX_AGENT_APP_SERVER_BINARY`，runner 会先做 binary preflight，避免把缺失二进制误报成 healthz 超时。

不能作为发布功能闭环证据的产物包括：旧版本/旧 HEAD/旧 runtime cache；旧工具
封锁口径；只看 `full_plugin >= 90` 或平均分；没有确认 runtime、backend、当前
case/snapshot、release identity 和同一 artifact；只跑 CLI/scorer/direct MCP；或
把 focused/cross-layer source PASS 冒充 Electron/formal package。正式结论必须
分别证明当前 Go host-owned dynamic catalog、EvidenceReceipt/ClaimRecord/Final
Evidence Gate、两个 typed local-display 产品入口、Provider、真实 Electron 和同一
artifact 的 A0/B1。旧版本产物不得冒充当前候选证据。

## 发布与更新

公开发布入口是 Analytix Hub 管理端的 Agent 插件路径（`https://analytix.top/admin-console`）。独立 `analytix-fund-analysis-marketplace` 仓库只是在 Hub 后台发布流程需要时作为版本化包源/检测源使用；`git push`、GitHub Release 或任意 GitHub remote 可访问性都不是客户侧公开发布入口，也不能单独作为发布成功或发布阻断判断。发布前需要同步插件目录、运行 validator 与健康检查，再通过 Hub 管理端发布或触发 Agent 插件检测更新。Analytix 插件 runtime 通过公开 `analytix-hub` marketplace 下载新包，安装缓存由 Analytix 自有 runtime 管理，不写入系统 Codex 的配置或缓存。

标准流程边界见 `references/hub-lifecycle.md`：发布只从 Hub 管理端 Agent 插件路径进入；如当前 Hub 工具链要求 marketplace 仓库，它只是 Hub 发布前的包源准备步骤。本仓库的 `sync-runtime-cache.mjs` 只用于本机 Analytix-owned runtime 验证，不是发布、安装或客户升级路径。安装由 Analytix Hub 将版本化包挂载到 Analytix runtime；卸载应通过 Hub/插件页移除挂载与 required skill；回滚应选择上一版本包并重新 mount；remount 只刷新 Analytix runtime 内的插件缓存和 skill 软挂载，不改系统 Codex 的 hooks、MCP、config 或 cache。

## 子代理策略

插件本身不强制启动子代理；是否启用由 Analytix 内嵌 Agent runtime 在支持子代理时执行。插件只规定策略：先取得当前案件的数据口径和统计树，再由 `plan_case_analysis` 作为复杂任务计划器决定是否拆分。单账户、单专题、少量证据切片默认不启用子代理；全案报告、多主体、多账户、多专题并行研判时，可以拆成账户画像、交易特征、主体 Top 出账续查、资金链路、现金/三方支付/虚拟资产、补证与 QA 等独立工作道。子代理只拿共享 scope pack、允许 MCP 工具和输出卡片要求，不接触账号密码、令牌或部署密钥。

## 报告输出

默认全案报告应先写数据来源、导入文件、数据清洗、最终分析口径，再按户名/主体展开账户集合、资金规模、进出账、交易特征、账户角色、资金链路、补证建议和结论性研判。涉及违法犯罪、非法获利、偷逃税等高风险判断时，只输出“线索”“疑似”“需复核”，不替代法律结论。
