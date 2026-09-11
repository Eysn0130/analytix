# Analytix 涉案资金研判插件认知增强落地执行方案

Status: Historical Funds-domain execution reference; not current controller authority
Current authority: repository/scoped `AGENTS.md`, accepted platform and Funds
specs, active authorized OpenSpec, current code, and fresh validation
Superseded for platform architecture and controller routing by:
`docs/analytix/specs/11-agent-platform-brand-and-architecture.md` and
`docs/analytix/README.md`

## 2026-06-30 analytix 案件项目历史执行说明

本执行方案已迁移到当前 `/Users/sun/Projects/analytix/plugins/analytix-fund-analysis`。旧 `/Users/sun/Downloads/Analytix_new` 只可作为历史参考，不再承载插件开发、运行、发布、证据或验收。

当前执行线不再围绕旧 `Analytix_new` 的“案件管理选中案件 -> backend active-case”机制。analytix 的正确执行链路是：

```text
当前 Codex/Agent 会话 workspaceRoot
-> 案件项目 .analytix/case-project.json
-> ~/Library/Application Support/analytix/data-analysis/cases/<case_id>/case.duckdb
-> runtime tool context 注入 _analytix.workspaceRealPath/threadId/turnId
-> analytix_funds MCP 根据当前案件项目解析 case source
-> 语义事实工具或 Controlled Case Workbench 只读查询当前案件 DuckDB
```

验收时必须覆盖公共 MCP 契约与唯一生产 `runtime-go` 的调用上下文。真实 analytix 桌面会话执行 `mcp__analytix_funds__*` 时必须收到完整 `_analytix.workspaceRealPath/threadId/turnId/caseId/caseBindingHash/datasetSnapshotId/contextEpoch/contextDigest` 及 ExecutionGrant 字段；缺失、过期或错配时必须在执行前 fail-closed。

任何工具、脚本、skill、文档、测试和发布流程都必须遵守当前案件项目隔离：一个案件项目只能读取自己的 DuckDB；另一个案件项目必须读取另一个项目绑定的 DuckDB；显式 `case_id` 只用于校验和诊断，不能覆盖当前工作区绑定到其他案件。

后端 API 不可达不等于案件未就绪。若 `.analytix/case-project.json` 与案件 DuckDB 均可解析，`get_current_case`、排行、统计、schema、Workbench 和 SQL 诊断应继续工作，并把后端不可达标为 warning。只有案件项目配置缺失、DuckDB 缺失、DuckDB 不可读、跨案冲突或安全策略阻断时，才返回 Case Source Blocker。

所有 MCP 工具复核必须分成两类：语义事实工具要尽量直接给出当前案件的可审计短事实；语义工具不足时，必须提供类似 `crystaldba/postgres-mcp` 的 DuckDB 现场勘查能力，包括 schema inspection、row count、profile、preview、EXPLAIN/diagnose、只读 SQL、query recipe、history 和 evidence card。Workbench 不是绕过插件，而是 analytix 受控 source connector 的升级路径。

资金穿透类语义工具不能只在后端服务可用时成立。`trace_subject_top_outflows`、`trace_fund_next_hop`、`trace_fund` 和 `build_fund_flow_graph` 必须在后端语义服务不可达时，直接回退到当前案件 DuckDB 的 `analysis_txn_detail_idx`，返回用户请求的有界 Top N、一跳终点分类、案内账户下一跳候选、query id、只读/当前案件/清洗分析范围验证摘要和穿透深度边界；只有后端和本地 DuckDB fallback 都失败时，才允许返回能力缺口。

本文档是 `top-pluginization-plan.md` 的执行方案，不替代蓝本。蓝本定义插件应达到的系统形态、能力边界和交付标准；本文档定义如何从当前失败状态出发，把 `Analytix 涉案资金研判` 插件改造成“让 Codex 更会想”的商业级能力层：事实来源受控，思考路径开放，source-backed 能力可发现，必要时有 Analytix-owned Controlled Case Workbench 兜底，输出后置复核，真实案件功能闭环可回放。

本文档不是教程、不是讨论纪要、不是发布说明，也不是给普通用户看的使用说明。它保留当时 Funds 施工的基准分解，但不是当前施工指令或唯一验收 authority。只有用户明确要求时才使用 Codex Goal；后文保留的 `/goal` 提示词不授权创建 Goal、扩大范围、提交、发布或作出验收结论。

## 0. 当前问题定性

### 0.0 Data Analytics-first 大改总令

本执行方案从本节开始不再以 Analytix 现有 frontdoor、单个 MCP 工具或固定链路修补为中心。后续施工必须先把官方 Data Analytics 插件的商业级产品骨架吃透，再把经侦资金分析领域化落地：

```text
Data Analytics 母版：index router -> source preflight -> focused workflow
-> live/source-backed verification -> validation state -> delivery contract

Analytix 领域化：case-context -> casegraph/fundgraph/source envelope
-> quick-fact / dossier / trace / lab / full-case / visual / report / workbench
-> data-quality / claim-review / evidence artifact
```

施工原则：

- `index`、`funds_investigate`、root skill 只做 router / navigator / capability discovery，不做总指挥，不垄断 rank/profile/trace/lab/report/workbench。
- `case_source_envelope` 是 Data Analytics preflight envelope 的经侦等价物，只提供当前案件、清洗范围、source-of-truth、质量状态、验证状态和交付状态，不替 Codex 决策。
- Analytix MCP 是当前案件 DuckDB 的默认语义事实层和受控 source connector；语义工具不足、结果冲突、用户挑战或任务明确要求自定义口径时，才升级到当前案件、清洗/analysis scope、只读、限行、purpose、validation、artifact 边界内的 SQL/Python/notebook/workbench。
- 不新增 Data Analytics 没有的工具禁令；只禁止无来源强答、弱来源冒充、未验证计算、固定样例污染、跨案事实污染、假交付和内部实现外泄。
- 性能路线是轻量默认、风险升级、交付前强验证：低风险 Quick Fact 不跑全案，金额争议/重复换卡/报告/图表/附件才升级 validation、workbench 或 delivery QA。
- 测试目标不是追分或做旧式对比实验，而是在真实问题中发现并修复与成熟插件相比的偏离，证明 Analytix 至少齐平 Data Analytics/Impeccable/CodeGraph/Superpowers/oh-my-codex 的对应机制，并在经侦资金分析场景超过它们的可迁移能力。

任何与本节冲突的旧 `/goal`、旧对比实验、旧打分、旧工具封锁口径、旧 evidence、旧 frontdoor 总控描述，应优先删除或降级为历史背景，不得继续驱动施工。

#### 0.0.0.1 MCP-first source lane 与受控 Workbench 升级序

施工线程必须把 Data Analytics 的 source connector / semantic layer 思路经侦化，而不是误解成“直接绕过插件裸查 DuckDB”。Analytix 的当前案件 DuckDB 由 Analytix 清洗模块、analysis index、casegraph/fundgraph 和 MCP runtime 共同治理；因此 `analytix_funds` MCP 是默认 source lane，不是可有可无的装饰，也不是最终答复 owner。

生产任务默认升级序必须落进 skill、tool schema、runtime、测试和真机验收：

```text
用户问题
-> focused skill 识别任务与交付深度
-> case_source_envelope 确认当前案件、清洗/analysis 范围、必要来源和缺口
-> MCP semantic fact layer 读取当前案件事实，返回短事实/候选/验证状态
-> Evidence Ledger 合并候选事实并选择 source-of-truth
-> 仅当语义事实不足、冲突、用户挑战、明确自定义口径或报告/图表需要可回放计算时，
   进入 Controlled Case Workbench / SQL / Python / notebook
-> focused skill 用公安经侦业务语言成稿，support/audit 留幕后
```

本节要求清除两类相反错误：

- 不能把 MCP-first 写成 MCP-only：`funds_investigate`、rank、card、frontdoor、graph 都不得成为唯一事实源、唯一首跳或最终中文答案 owner。
- 不能把 Data Analytics 的可复核分析能力写成 raw-analysis-first：普通案件事实不应先让模型用 shell 搜索本地 DuckDB、历史线程、旧 evidence 或工作目录；SQL/Python/notebook 只能作为当前案件、只读、清洗/analysis scope、限行、purpose、validation、artifact/evidence boundary 内的升级路径。

Top-N 是用户意图合同，不是 UI 预览常量。凡用户明确要求“前十位/Top 10/前 N 位”，语义排行工具、Top 出账 payload、frontdoor destination 卡、Workbench compact text 和 oracle 都必须以请求的有界 `limit/top_n` 为准；如需排除同主体/自转对手方，前门必须抓取 buffered rank rows 后再过滤，不能因为默认预览或过滤后不足而只给 5 位或 9 位。若语义工具仍不足，`diagnose_case_sql` 必须给出 SQL-safe column/table 候选和当前案件 `analysis_*`/`fc_*_norm` 修复路径，不能把可执行 identifier 翻译成用户正文词。

对应施工项：

1. 审计所有 skill、MCP description、tool schema、runtime 输出、answer compiler、eval 和 docs，删除“只能 MCP”和“禁止 SQL/Python/notebook/DuckDB”的错误表述，改成“语义事实层优先，Workbench 受控升级”。
2. MCP 输出只给模型短事实、候选、source map、validation state 和可用证据线索；不得输出完整最终稿、强制标题或用户可见工程词。
3. Workbench 成功结果必须回填 Evidence Ledger，与 MCP 候选事实 reconciliation；失败只能给 source blocker，不能让模型从弱来源补数。
4. `shell` 仅用于插件工程、打包、报告/图表 QA、artifact 检查和开发验证；普通案件事实链路不得通过列目录、搜索本地数据库或旧会话猜案。
5. 真机测试必须覆盖：MCP 足够时不再重复 SQL；MCP 不足/冲突/被用户挑战时能升级 Workbench；两条路径结果冲突时能解释 source-of-truth，而不是混用。

#### 0.0.1 MCP 去中心化施工原则

本轮必须把 “只能走某个 MCP 读取 DuckDB 并返回答案” 视为必须裁剪的错误路线，而不是继续围绕单个 MCP 补洞。正确方向是 Data Analytics 母版 + 经侦领域化：skill 负责理解任务和 workflow，`case_source_envelope` 负责来源信封，Codex 自主选择 source-backed 路径，MCP 只承担 source adapter、governed executor、semantic map helper、validator、delivery/artifact bridge。

施工线程必须先清点所有 MCP、runtime、tool schema、tool descriptions、metadata、registry、skill 文案和 eval：

| MCP 类型 | 处理 |
| --- | --- |
| 直接生成最终中文研判答案 | 从 production tool discovery、tool schema、skill reference、answer contract 和普通 eval 中删除或禁用；最终回答交给 Codex + focused skill。 |
| 直接读取 DuckDB 并写死业务口径 | 从 production 默认面删除；只允许迁移为 reference 指引、query guidance 或 workbench template。 |
| 大 JSON、raw rows、bounded preview | 从模型可见默认面删除；只允许进入 artifact / `_meta` / ledger。 |
| 排名工具控制金额事实 | 删除其最终 claim authority；只有标 `ranking_only` / `needs_review` / `candidate` 且不控制 Pair Amount 时才可保留。 |
| eval/golden/diagnostic/report-gate 输出 | 从 production resources、skill references、用户输出和普通 tool discovery 删除；只留 eval/dev/release audit。 |

判断标准不是“有没有 MCP”，而是“MCP 是否替 Codex 做了任务理解、路径选择和最终结论”。若是，直接砍出 production 路径；若 MCP 只提供来源、执行、验证、artifact 和 provenance，才允许保留并强化。

#### 0.0.2 二次污染防线施工原则

施工线程必须专门审计“MCP/helper 返回一个口径，Codex 后续又查 DuckDB/workbench 得到另一个口径”的二次污染风险。Data Analytics 的正确做法是 semantic layer/source helper 只做地图和低成本候选，所有 live source reads、SQL/notebook、图表、报告 claim 都进入同一个 evidence/provenance 体系。

施工要求：

- MCP/helper 输出默认改为 source map、query guidance、compact evidence、validation state、provenance、known risks，不默认写最终中文研判；做不到这一点的 MCP 直接删除或禁用 production 默认面。
- 所有聚合结果必须带 scope、grain、dedup、time_window、source_hash、query_id、validation_state；`rank_counterparties` 等排行只可作为 ranking/candidate，不能控制 Pair Amount。
- Workbench/SQL/Python/notebook 的结果必须回填 Evidence Ledger，与 MCP/helper 候选事实合并、去重、冲突标注。
- 同一 claim 多结果时，必须做 source-of-truth selection；无法解释差异时输出 Claim Gap / Reconciliation Needed，不能取最大值、取最新值或相加。
- Skill/reference 要指导 Codex 看哪些清洗表/analysis index、常见口径坑、重复/换卡/同事实风险和验证方法，而不是要求模型相信某个固定 MCP 返回。

#### 0.0.2.1 三插件限制对齐施工合同

本轮继续对照 Data Analytics、Investment Banking、Public Equity Investing 后，必须把“限制”分成两类。Analytix 只继承成熟插件已有的 source / validation / delivery / support-hidden 边界，不能自创会压制 Codex 研判能力的工具封锁或固定路径。

**必须保留的限制**：

- 当前案件和 source-of-truth 不明时，不给报告级金额、链路、资产、关系或法律敏感确定结论。
- 弱来源、样本行、排行、bounded preview、旧报告、旧线程、fixture、历史图片或本地文件不能冒充当前 DuckDB 清洗表/analysis 索引事实。
- raw rows、大 JSON、support envelope、workflow、case_id、debug、handoff、`answer_draft`、`delivery_state`、`direct/candidate` 等内部材料不得进入普通模型正文或用户答案。
- SQL/Python/notebook/DuckDB/shell 只能在当前案件、只读、清洗/analysis scope、限行、purpose、validation、artifact/evidence boundary 内支撑结论；不得越权读写、跨案 join、访问外部文件/网络/secret 或绕过 Analytix 清洗边界。
- 报告、图表、notebook、附件、导出文件只有在 Analytix/Codex 可用交付面真实生成并检查后才能说已交付；没有交付面或检查失败时必须写交付缺口。
- 法律定性、实际控制、资金性质、最终去向、资产权属和犯罪事实只能在证据支持范围内表达为事实、线索、需复核或暂不能认定。

**必须清除的错误限制**：

- 禁止把“优先语义工具”写成“不得 SQL/Python/notebook/DuckDB/shell”。Data Analytics 允许这些路径，Analytix 也允许；区别只是它们必须受当前案件和可复核边界管理。
- 禁止把 `funds_investigate`、rank、frontdoor、card 或某个 MCP 写成普通问题唯一首跳、唯一事实源或最终答案 owner。
- 禁止把 `answer_card_complete`、`max_additional_tools`、stop condition 写成跨轮硬停止。它们只约束当前轮无效追加调用，不压制改口径、继续追一层、否定性搜索和开放假设。
- 禁止把“必要边界”写成机械长清单。边界应转成经侦业务语言，并服务于结论、表格、异常、资金意义和补证动作。
- 禁止用测试分数、direct MCP 正确、CLI 脚本通过或旧 A/B 产物替代真实 analytixagent 前门多轮质量。
- 禁止因为语义 MCP 暂时不足就拒绝当前明确问题；应像 Data Analytics 一样先换 source-backed 路径，必要时用 Workbench/SQL/notebook 得到当前 bounded 结论，并把可产品化的新语义工具作为后续工程项。

施工线程审计所有 `不得/禁止/只能/只允许/stop` 时，必须先判断它是在保护事实、证据、权限和交付，还是在限制 Codex 的工具选择、侦查假设和成稿能力。前者保留并做证据化；后者删除或改成软边界。

**插件能力边界修正**：

- 边界内：显式授权的 `export_cleaned_case_data` 清洗 CSV/XLSX 导出；基于已核事实的表格、图表、Mermaid/PNG/JPG/workbook/report figure 交付契约和质量检查；结构化 `FinalInvestigationAnswer`、证据边界、补证动作和内部审计事件。
- 边界外：插件自行实现通用 PDF 渲染器、OCR 流水线、资产登记接口、任意文件导出系统或外部证据获取系统。开户资料、回单、票据、合同、OCR 文本、资产登记等只能作为补证材料请求，或作为用户/Analytix 后端已经提供的附件事实处理。
- 因此后续 P1/P2/P3 不再写“通用 PDF 渲染闭环、OCR/资产登记链路”这类系统建设口径；改为补强真实交付面的验收合同、证据引用 canary、validation/diagnose/delivery-qc 审计事件，以及补证建议结构。

#### 0.0.2.2 Skill Creator + 成熟插件结构化落地合同

本轮改造必须把 Codex/Anthropic Skill Creator、Data Analytics、Investment Banking、Public Equity Investing 的核心机制落到 Analytix 资金研判插件，而不是只在蓝本里引用名称。施工线程按以下四层改：

| 层 | 必改项 | 验收 |
| --- | --- | --- |
| Skill 结构层 | root/index 只负责发现和 lead selection；focused skill 使用短 `SKILL.md`、清晰 frontmatter description、Use/Do Not Use、Workflow、Completion Gate；经侦方法、术语、表格/报告样式放一跳 reference；确定性检查进入 scripts。 | 真实金额、主体、流向、报告问题能触发正确 owner；`SKILL.md` 不膨胀成蓝本；production skill 不含 eval/golden 答案和真实案件事实。 |
| Source preflight 层 | `case_source_envelope` 对齐 Data Analytics user-context：确认 current case project、清洗/analysis 覆盖、必需来源、可选 enrichment、质量缺口、交付要求和当前可用 source-of-truth。 | case-project source 缺失 fail-fast；可选来源缺失不阻断当前可答问题；用户正文不以 preflight 开头。 |
| Semantic + live verification 层 | MCP 语义事实层只做 source adapter、semantic map helper、validator 和 artifact bridge；重要 claim 通过 MCP fact 或受控 Workbench live verification 后进入 Evidence Ledger。 | rank、profile、样本行、preview 不得单独控制报告级金额/链路；同一 claim 多来源必须 source-of-truth selection；冲突不可解释时输出 Claim Gap。 |
| Controlled Workbench 层 | SQL/Python/notebook/DuckDB 是正常 source-backed 能力：语义事实不足、口径冲突、用户挑战、自定义计算、导出明细、图表/报告可回放时进入；使用 current-case、read-only、cleaned/analysis scope、row limit、purpose、validation、artifact/evidence boundary。 | 不再出现 blanket forbidden；也不让模型默认首跳裸查本地数据库、历史文件或工作目录；Workbench 结果必须回填 Evidence Ledger。 |
| 专业交付层 | 借鉴 IB/PE 的 lead workflow / hero artifact first：金额核验、主体画像、资金穿透、全案报告、图表/附件由 focused owner 成稿；support/audit 全部留幕后。 | 用户最终看到公安经侦材料：结论、事实表、异常特征、资金意义、证据成熟度、暂不能认定事项和补证动作；看不到 routing、workflow、case_id、support JSON、SQL 过程说明。 |

新增或调整代码/文档时，优先落到这些承载点：`skills/*/SKILL.md`、`references/focused-skill-shared.md`、`references/case-workbench*`、`mcp/tool-schemas.mjs`、`mcp/agent-output-compiler.mjs`、`mcp/card-renderer.mjs`、`mcp/evidence-ledger*`、`scripts/*contract*` 和真实 UI functional closure。不能只改 prompt 文案而不改 source/evidence/delivery 结构。

#### 0.0.2.3 当前案件 DuckDB 分析完毕后再续调

“续调”必须是当前案件数据分析到边界后的证明动作，不能作为事实不足、工具不足或答案空泛时的口头兜底。施工线程必须把该门槛写入 skill completion gate、answer compiler、claim review 和 functional eval。

分层要求：

1. **普通短问**：不要求全库重算，但必须在当前问题范围内先查明 current case project、相关清洗/analysis 来源、统计期间、对象口径和必要边界。若可由当前案件事实回答，直接 answer-first；不得先写泛化续调。
2. **一跳金额 / 金额挑战**：先核查当前案件 DuckDB 中相关付款/收款账户、入账环节、集中日期、大额交易、同事实候选、重复/换卡、手续费/本金口径、缺失对手方和下游可见 Top 去向；当前数据不能证明下一层时，再列续调对象和材料。
3. **主体画像 / 账户研判**：先核查登记账户、待核账户线索、进账/出账规模、重点账户、重点对手方、时间/金额集中、现金、理财/证券/基金、资产消费、支付机构、设备/IP/MAC/联系方式等当前数据支持线索；缺哪一类证明，就续调哪一类材料。
4. **资金穿透 / 流向图**：先用 casegraph/fundgraph、trace、rank 和必要 Workbench 汇总当前可见上游来源、主链、下游去向、回流、断点和旁路线索；只对未调取账户、缺回单、缺余额承接、缺产品持仓/赎回、缺支付机构明细、缺资产登记等数据断点提出续调。
5. **全案报告 / 正式材料**：先完成当前案件数据盘点、主体/账户/对手方盘点、资金规模、重点链路、异常专题、图表/附件、claim review 和 delivery QC；正式续调清单必须按对象、账号/户名、期间、材料、证明目的、对应证据缺口列出。

验收规则：

- 只基于 rank/card/样本行就写“建议进一步核查”的，判 fail。
- 当前案件可得数据未分析到用户问题范围内的数据边界，就给资金穿透续调建议的，判 fail。
- 续调建议必须具体到材料与证明目的：例如银行回单、开户/KYC、账户余额承接、对手账户后续流水、理财/证券/基金持仓与赎回、第三方支付明细、资产登记或消费凭证；不得只写“继续调取相关资料”。
- 当数据不足以认定资金性质、实际控制、最终去向、资产权属或犯罪事实时，只能写“线索 / 需复核 / 暂不能认定”，并说明需要什么证据把线索升级为事实。
- 相关测试至少覆盖“问金额 -> 画流向图 -> 研判收款主体 -> 继续追下游 -> 生成材料”，检查是否先充分使用当前案件 DuckDB，再提出具体续调动作。

#### 0.0.3 2026-06-09 真机 P0 事故闭环

本节记录本轮真实 analytixagent 前门暴露的结构性失败。具体真实案件姓名、金额和逐笔明细只能进入本轮 `functional-closure/` 产物、人工真机记录或用户授权的新线程 prompt，不得写入 production skill、progressive resource、runtime answer contract、Hub package 默认资源或普通用户可见帮助文本。

事故表现：

- `rank_counterparties` 和 `get_current_case` 首跳失败，错误为 `Unable to resolve analytix case project via current case-project binding: current case project is not set`。
- Agent 随后列目录、搜索 `*.duckdb`、尝试历史或测试残留 `case_id`，产生 `case not found`，并重复调用 `rank_holders`。这不是“模型勤奋”，而是 required source-of-truth 缺失后没有 Data Analytics 式 source guardrail。
- analytix 案件项目会通过工作区上下文进入 Agent；插件和 MCP runtime 不能把 current case-project binding 为空解释成“用户没选案件”。正确产品行为是：Analytix 主系统在启动、进入 Agent、切换案件、恢复上次案件和 runtime bootstrap 时，把当前打开/上次恢复案件同步到 current case-project binding；MCP 只消费 case-project source 或显式 `case_id`，不得扫描本地案件目录猜测。
- Pair Amount / Amount Challenge 中，Workbench SQL 错把占位交易号当作可靠唯一交易号，导致不同交易被合并，输出错误 effective amount；这说明 validation/critique 没有覆盖去重键安全性和算术一致性。
- 最终答复只给金额摘要，没有展开两方金额核验材料的相关账户（列明账号）、时间集中、重复/换卡组、差异来源、下一跳和可疑特征，说明 focused skill completion gate 未在真实前门生效。

P0 修复合同：

1. **当前案件同步 fail-fast**：无显式 `case_id` 时，只允许从 current case-project binding 或 Analytix-owned runtime context 解析当前案件；若 case-project source 为空但主系统有上次打开案件，先自动同步；若仍为空，返回 Case Source Blocker 和恢复动作。禁止列目录、搜索本地 DuckDB、从 Downloads/历史输出/测试 fixture 猜 case_id。
2. **last-opened case 同步**：施工必须找到 Analytix 桌面端“启动默认打开上一次案件”的真实状态源，打通 `case project workspace -> .analytix/case-project.json -> analytix runtime _analytix.workspaceRealPath -> analytix_funds`。验收要覆盖冷启动、刷新、切换案件、新建 Agent 会话和 runtime cache remount。
3. **显式 case_id 冲突治理**：显式 `case_id` 优先，但必须验证存在；不存在时不再尝试下一个猜测 id，直接返回 invalid case blocker。case-project source 与显式 case 冲突时写 scope warning。
4. **Pair Amount 去重键安全**：`txn_id` 为空、`查无信息`、`unknown`、`无`、`null`、重复占位、系统填充值或明显非唯一值时，不得单独作为 fact key。有效交易号只能作为候选键的一部分；无有效交易号时必须使用时间、金额、方向、余额、对手账号、解析对手户名、摘要/备注、成功状态等复合键，并保留重复来源账户列表。
5. **金额统计范围四分法**：Pair Amount 输出必须分列 `raw_detail_amount`、`high_confidence_same_fact_effective_amount`、可选 `principal_amount_after_fee_exclusion`、`duplicate_or_unsupported_amount`，并说明每一项控制来源和适用条件。不能把候选重复金额静默扣减为“最终金额”，也不能把排行金额写成 Pair Amount。
6. **final critique 必过**：金额挑战最终答复前必须做轻量 critique：算术是否闭合、去重键是否含占位、raw/effective/本金口径是否混用、是否解释竞争金额、是否给账户/时间集中和下一步追踪。失败时不许交付确定结论。
7. **真实前门回归**：功能闭环必须增加本事故同类任务，但真实姓名/金额只能在本地 closure 产物或环境变量中，生产 reference 只保存结构性断言：不得接受错误 effective amount；必须解释排行金额、raw 明细、同事实去重、手续费/本金口径和下一跳；必须证明 case-project source 不是靠扫描本地文件获得。

#### 0.0.4 2026-06-10 用户交付层 P0 事故闭环

本节记录 019eafd0 后续真机多轮验收暴露的新问题。它不是宿主 Codex 过程卡片问题，而是插件 production 输出把内部审计合同、测试合同和工具边界说明直接交给模型，导致最终答复像说明书、低质 AIGC 或内部流程复述。该问题属于插件本体 P0，必须在发布前闭环。

事故表现：

- 用户问“主体甲转给主体乙多少钱？”时，最终回答出现当前案件预检、同名聚合内部标题、核心收款账号内部标题等工具化首句。用户已经在当前案件内提问，普通研判不应先解释工具统计边界，而应 answer-first 写成经侦研判材料：经梳理，某期间、某主体向某主体转账多少笔、合计多少元、主要集中日/账户/大额交易、异常线索和下一步核查。
- 用户追问“画出流向图”时，最终回答按“口径与来源边界、数据事实、已有数据支持的资金边、统计特征、线索、需复核、证据缺口、不可判断”机械分段。这些是内部审计/验证状态，不是资金穿透图的读者结构。用户要看资金来源、主链、下游去向、断点和补证建议。
- 用户问“对主体乙名下账户进行研判”时，最终回答出现 `Subject Dossier / 主体画像`、`归属边界（direct / candidate）`、`候选线索账户 0 个` 等英文或审计标签。主体画像应按公安经侦口吻输出账户基本情况、账户清单、资金流入流出、重点账户、重点对手方、异常特征、可疑去向和核查建议。

根因定位：

- `destination-diagnostic-runtime.mjs` 曾把金额核算内部 label 作为可见事实 label，例如同名收款人聚合标题、核心收款卡/账号标题。这些 label 可留在 `_meta`、ledger 或 debug，不应作为普通用户答案标题。
- `agent-output-compiler.mjs` 正在拼接大量“必须原样保留”“最终回答必须保留这个小标题”“不要合并或改名”等指令型文本。MCP/card 的 agent-readable 摘要应该给事实和风险，不应该给最终用户标题合同。
- `graph-visualization/SKILL.md` 和 `fund-flow-graph-runtime.mjs` 将图谱审计结构固化为用户可见标题，导致资金流向图变成验证清单，而不是资金穿透材料。
- `subject-dossier/SKILL.md` 和主体画像输出把 `direct/candidate` 作为用户可见分类名。内部可区分直接登记账户和线索账户，但普通用户应看到“已登记账户”“待核账户线索”“不能直接认定为名下账户”等中文业务表达。
- 当前测试主要检查“有没有保留内部边界词”，没有检查“最终答复是否像经侦材料、是否少说无关口径、是否对用户目的展开分析”。

施工合同：

1. **审计层与交付层分离**：source、scope、validation_state、candidate、unsupported、dedupe、delivery gate 等可以进入 `_meta`、ledger、doctor、eval、debug artifact；普通用户正文只保留必要统计范围差异、证据边界和补证建议，且必须转写为经侦业务语言。
2. **禁止“必须原样保留小标题”进入用户路径**：production agent-readable text 不得出现要求模型原样保留审计标题的句子。若测试需要固定结构，只能检查语义存在，而不是强迫标题外露。
3. **Answer-first 默认**：普通事实题先给结论，再给必要表格和研判要点。不得用工具预检句开头；不得把 source preflight 当用户首句。
4. **报告式表达模板**：
   - 一跳金额：`经梳理，YYYY年MM月DD日至YYYY年MM月DD日期间，A 向 B 转账 N 笔，合计 X 元。主要集中于...；其中需说明...；研判意见...；下一步...`
   - 资金流向图：`资金来源/主链/下游去向/资金断点/待补证事项`，图中只画可证实资金边，旁路线索用注释或待核列表表达。
   - 主体账户研判：`账户基本情况/账户清单 Top/资金流入/资金流出/重点对手方/异常特征/疑点分析/核查建议`。
5. **深度展开义务**：用户问金额、账户或流向时，插件不能只答一个数字或审计标签；应在不越证据边界的前提下给出 Top5/Top10 表格、时间集中、大额交易、夜间/集中/理财/证券/购房购车/现金/缺失对手等可疑特征，以及下一步补证方向。
6. **必要边界短写**：同事实/换卡、同名账户、缺对手字段、未取得回单、未做余额承接等边界要写，但用一句业务化说明即可，不能铺成“质量边界/禁止写成事实/下一步复核”长清单。
7. **最终回答质检**：`analysis-critique` 或等价轻量质检必须检查：是否机械、是否出现内部标题/英文标签、是否 answer-first、是否有表格/图谱/研判意见、是否解释金额差异、是否提出下一步核查。

新增负向测试必须禁止普通用户最终回答出现：

- 当前案件预检式开头，把工具输入边界当结论首句。
- 同名收款人聚合、核心收款账号等内部统计标题。
- 来源边界、数据事实、图谱依据表等审计清单标题。
- 英文对象画像标题或 `direct/candidate` 等内部分类原词。
- 候选账户数量为零一类机械化诊断句。
- `必须保留`
- `不要合并或改名`

新增正向测试必须覆盖：

- 一跳金额问答：answer-first，给期间、笔数、金额、主要集中日、账户/对手账号数量、Top 大额交易表、异常/待核点和下一步核查；只在必要处简述不同金额统计范围的差异含义。
- 资金流向图：输出可读资金穿透图或图表，包含上游来源、主链、下游理财/公司/回转/缺端点断点；标题用“资金流向研判”“主要资金链路”“待补证事项”等。
- 主体账户研判：给账户基本情况、登记账户口径、Top 账户表、进账/出账概况、重点对手方、异常特征、可疑用途、核查建议；不得出现 direct/candidate 英文。
- 全案/报告/图表：报告必须像公安经侦材料，表格和图谱服务研判结论，而不是服务审计合同。

#### 0.0.5 2026-06-11 金融专业插件判断层 P0 事故闭环

本节吸收本机已安装 `Investment Banking` 与 `Public Equity Investing` 插件的成熟做法，修正 019eafd0 后续真机暴露的新失败：Analytix 已能返回若干金额和口径，但最终答复仍像 fact-card 摘要，缺少专业研判层、读者交付层和案件意义判断。该问题不能继续用“替换内部词”“再加一个事实工具”解决，必须补齐金融专业插件共同具备的判断层。

本机成熟插件研究结论：

- `Investment Banking` 的 root skill 只做 invocation gate、user-context preflight 和 lead skill selection；router 不做实质业务、不选择最终展示面、不把 internal support 当用户可见技能。选定 lead skill 后，deliverable intake、格式、深度、artifact hierarchy 和 final response 由 lead workflow 负责。
- `Investment Banking` 的 routing playbook 要求“一个 lead skill 负责第一层真实判断或 hero artifact”，support skills 只服务 source normalization、model audit、deck QC、style 等 owned workstreams；用户看到的是 board/client/committee-ready 交付，而不是路由、handoff JSON 或内部支撑包。
- `Public Equity Investing` 的 final deliverable framework 要求 human-readable investment artifact first，CSV/JSON/Markdown/logs/manifests 只能做 support/audit files；chat 只是 cover note，不能替代 hero artifact。
- `Public Equity Investing` 的 PM judgment heuristics 要求每个实质输出回答“mispriced / priced-in / proof / kill thesis / why now / action / missing evidence”等判断问题；这不是多跑数据，而是把已验证事实转成可执行的专业判断。
- `Public Equity Investing` 的 support-layer routing contract 明确 support service 不拥有最终投资结论，必须把 `decision_impact`、`readiness_effect`、source/QC issue 转成用户能理解的影响、 caveat 或 next source，而不是外露内部字段。

Analytix 当前对应失败：

- `funds_investigate` / 金额事实卡仍在承担 final answer owner，Codex 只把它压缩成“两个口径、不能相加”，没有进入经侦判断层。
- Quick Fact 被误实现成“查数说明”，不是“两方金额核验材料”。用户问一跳金额时，插件只给数值和口径，没有自然补齐大额交易表、时间/账户集中、异常特征、资金用途线索、关系/角色判断、后续追踪路径。
- 图谱、主体画像和报告仍可能让审计标题支配读者结构，缺少类似 IB/PE 的 hero deliverable first 和专业读者路径。
- 当前正向测试偏重“有没有禁用内部词”，不够检查“是否有研判意义、是否回答 so what、是否能作为办案材料继续使用”。

新增施工合同：

1. **Lead workflow owner**：`quick-fact`、`subject-dossier`、`fund-tracing`、`full-case-analysis`、`visual-evidence`、`report-builder` 必须像 IB/PE focused workflow 一样拥有最终用户交付责任。`index`、root skill、`funds_investigate`、source helper、workbench、validator 只能是 router/support，不得成为最终中文研判 owner。
2. **经侦判断层**：每个实质回答在 source-backed facts 之后必须形成 `investigative_judgment_layer`，至少回答：这笔/这组资金在案件里有什么意义、是否异常、异常在哪里、还不能证明什么、下一步查什么。普通用户不看字段名；程序可在 `_meta`/ledger 保存结构。
3. **一跳金额最小专业交付**：`A 转给 B 多少钱` 不得只给两个金额统计范围。合格答复必须包含：结论句、统计期间、笔数和金额、主要集中日/集中账户、大额交易 Top 表、竞争金额的业务含义、重复/换卡/同事实风险、异常特征扫描、关系/角色或资金目的线索、下游追踪建议。若用户明确只要一句数字，才压缩为结论 + 一句边界。
4. **异常特征扫描默认轻量化**：在不重跑全案的前提下，从已取得的一跳/主体/图谱事实中扫描时间异常、连续拆分、整额/大额、短时密集、同日多笔、多账户分散、回流、理财/证券/基金、购房购车/大额消费、现金、缺失对手、设备/IP/MAC/开户地址/联系方式关联线索。没有证据就写“未在本轮事实中支持”，不得编造。
5. **Hero deliverable first**：报告、图谱、表格、附件、账户画像必须以用户可读交付物为主。support JSON、ledger、SQL、审计记录、workflow、case_id、supported seed、delivery_state 等只做 support/audit，不得成为正文结构。
6. **专业口吻与公安经侦材料化**：输出先写“经梳理/经统计/研判认为/建议补调”，再给事实表和分析；避免当前案件预检、同名聚合支持、核心账号统计等支撑层首句。必要统计范围差异转写成“统计范围不同，因此 X 元是已确认核心账号收款，Y 元是同名收款人合计支持金额”这类业务语言。
7. **QC 从词汇升级到判断质量**：`analysis-critique` 不只查内部词，还要查是否有 Top 表、是否解释资金意义、是否有异常扫描、是否有下一步侦查动作、是否把支持层当成结论、是否把“统计范围不同”写成用户看不懂的技术句。

新增测试与验收：

- `professional_judgment_contract`：断言 Pair Amount、主体研判、资金流向图、全案报告片段都包含结论、事实表、研判意见、异常特征、下一步核查；否则 fail。
- `support_layer_hidden_contract`：断言用户最终回答不出现 support-layer 字段、工具 routing、workflow、case_id、supported seed、debug label、delivery_state、internal handoff。
- `pair_amount_mini_investigation_smoke`：同一 Pair Amount 问题必须输出经侦口吻两方金额核验材料，并包含 Top 大额交易表和异常扫描。只输出“两个口径，不能相加”判 fail。
- `hero_delivery_contract`：图谱/报告/账户研判必须以资金流向图、账户梳理表、研判报告、补证清单作为 hero deliverable；聊天摘要不得冒充正式交付物。
- 真机多轮验收必须包含“问金额 -> 画流向图 -> 研判收款主体 -> 继续追下游 -> 生成材料”的连续场景，检查是否像专业经侦助手，而不是工具说明员。

#### 0.0.6 2026-06-11 成熟插件结构化重构 P0：support 层退场、focused owner 成稿

0.0.5 补齐了专业判断层，但还不足以解释 019eafd0 真机为何仍会复述“两方金额核验材料”“两个口径不能相加”。继续深挖 Data Analytics、Investment Banking、Public Equity Investing 后，新的根因判断是：Analytix 仍让 support 层生成用户可复述的答案草稿，focused skill 只是被动包装。成熟插件正好相反：router 只选 lead workflow，source/helper 只给 evidence，最终中文交付由 lead focused skill 负责。

本轮静态审计必须优先检查并修复这些已知偏差：

- `skills/quick-fact/SKILL.md` 仍可能要求 Pair Amount 先调用 `funds_investigate`，使用其 `answer draft` 后停止。这会绕过金额核验 owner。
- root compatibility skill 仍可能要求普通资金问题首个内部动作调用 `funds_investigate`，这会把 navigator 重新变成总前门。
- `mcp/tool-schemas.mjs` 若继续描述“普通桌面问法应优先使用 `funds_investigate`”，会让模型优先选自然语言事实卡，而不是 focused semantic tool。
- `mcp/card-renderer.mjs`、`mcp/frontdoor-runtime.mjs`、`mcp/agent-output-compiler.mjs`、`mcp/context-compiler.mjs` 若继续把 `answer_draft`、卡片标题和段落作为 agent-readable 主体，就会让 Codex 复述 support 文本。
- `capability-registry.json`、`command-metadata.json`、doctor、functional eval 若仍以 `funds_investigate` 为普通问题上游事实源，会让测试证明旧前门，而不是证明 focused owner。

结构化重构合同：

1. **新增或落实金额核验 owner**：建立 `pair-amount-investigation` focused skill，或把等价 owner 明确落在独立 skill / workflow 中；`quick-fact` 只处理低风险单点统计和排行，遇到 A->B 金额、金额挑战、重复/换卡/同事实风险必须转交该 owner。
2. **`funds_investigate` 降级为 support navigator**：只返回 `next_owner_skill`、`source_map`、`compact_evidence`、`query_guidance`、`validation_state`、`known_risks`、`support_facts`。不得再输出默认可复述的最终中文研判；若保留 `answer_draft`，必须默认隐藏在 audit/support，不进入普通 agent-readable 主体。
3. **卡片编译器退出成稿位**：card renderer / output compiler / context compiler 只输出证据摘要、验证状态、风险和下一步 owner；不得生成“两方金额核验材料：A向B一跳转账”“金额统计范围”“证据边界”“补证建议”等用户答案式段落。
4. **focused owner 承担公安经侦成稿**：`pair-amount-investigation`、`subject-dossier`、`fund-tracing`、`visual-evidence`、`report-builder` 的 `SKILL.md` 必须有 Use When、Workflow、Output Contract、Completion Gate，并说明如何把 support facts 转成办案材料，而不是复制工具输出。
5. **语义层只做地图**：rank/profile/trace/fundgraph/casegraph/workbench 可以给候选、证据、聚合和 artifact；最终 claim 必须由 owner 汇总 source-of-truth、validation state 和 investigative judgment 后输出。
6. **质量门查结构而不只查词**：新增或扩展 `support_layer_hidden_contract`、`lead_owner_contract`、`funds_investigate_not_final_owner`、`quick_fact_no_pair_amount_owner`、`pair_amount_owner_smoke`。测试必须断言用户最终回答不是 MCP/card 原文摘要。
7. **经侦领域继续扩展但不制造总控**：若发现 IP/MAC、联系电话/住址、资产消费、理财证券、现金桥、境外/虚拟资产、票税合同、团伙关联等覆盖不足，应扩展 `investigation-lab`、`relationship-network-analysis`、`asset-product-analysis` 或 source helper；不得把所有专题塞回 `funds_investigate`。

完成判据：

- Pair Amount 真机最终答复由金额核验 owner 成稿，包含结论、期间、事实表、异常扫描、竞争金额业务解释和下一步核验；`funds_investigate` 只能作为候选支持。
- 对 production skill/runtime/schema/metadata 的 `rg` 静态扫描不得再出现“普通桌面问法应优先使用 `funds_investigate`”“首个内部动作应调用 `funds_investigate`”“Use its answer draft then stop”等成稿合同；蓝本/执行方案可以保留这些短语作为事故记录和反模式说明。
- `answer_draft` 不再作为普通 agent-readable 主体；如果仍存在字段，必须被命名、编译、测试为 support/audit-only。
- 真实桌面端“问金额 -> 画流向图 -> 研判收款主体 -> 继续追下游 -> 生成材料”不外露 workflow/case_id/debug/support 字段，且不复述事实卡小标题。

#### 0.0.7 2026-06-11 经侦专家交付层 P0：从正确数据到可办案材料

0.0.6 解决“谁成稿”的结构问题，本节解决“成稿质量像不像经侦专家”的产品问题。继续对照 Data Analytics、Investment Banking、Public Equity Investing 后，Analytix 不能只满足事实正确、口径可核、内部词不外泄；它必须让普通办案用户拿到可读、可用、可追查的公安经侦研判材料。

成熟插件对应机制：

- Data Analytics `build-report` 要求 report spine、reader-facing structure、每个重要数字都有解释和 implication；图表/表格必须支持一个 claim，不是装饰。
- Data Analytics `product-business-analysis` 要求先识别 decision/action，再把量化证据转成 recommendation、uncertainty 和 follow-up。
- Investment Banking `memo-builder` 要求 memo plan、decision hinge、load-bearing claims、source posture、open items、recommended next step；support artifacts 不得成为 hero deliverable。
- Public Equity Investing `memo-builder` 要求 decision ask、what must be true、disconfirmers、monitoring、source posture 和 PM judgment；薄数据只能形成 screen-grade skeleton 与 upgrade path，不能假装成熟。

Analytix 经侦化施工合同：

1. **新增案件研判 spine**：所有实质回答先形成内部 spine：`研判对象 -> 直接结论 -> 控制口径 -> 关键事实 -> 异常特征 -> 资金意义/角色关系 -> 证据缺口 -> 下一步侦查动作`。用户正文可以压缩，但不能缺少该判断链。
2. **新增研判材料 plan**：报告、全案、主体画像、金额核验、流向图、专题研判都要记录材料用途、读者、案件范围、来源成熟度、关键金额/账户/链路、待核事项、可报送状态。plan 留 support/audit，不在正文裸露。
3. **新增证据成熟度表达**：用户正文使用“已有流水支持 / 高可信支持 / 线索 / 需复核 / 暂不能认定 / 可形成材料候选”等经侦语言；禁止 `screen-grade`、`support layer`、`delivery_state`、`direct/candidate` 等工程/金融插件原词。
4. **修正核心交付模板**：
   - Pair Amount：经梳理 + 期间 + 金额/笔数 + 大额交易表 + 账户/时间集中 + 竞争金额业务解释 + 异常特征 + 关系/资金目的线索 + 下游追踪建议。
   - 主体/账户研判：基本情况、账户清单、资金概况、重点账户、进/出 Top、可疑特征、资金来源/去向、资产/理财/现金/境外/支付/设备/联系方式线索、核查建议。
   - 资金流向图：资金来源、主链、下游去向、资金断点、旁路线索、补证事项；不得用“口径与来源边界/数据事实/已有数据支持的资金边”作读者结构。
   - 全案研判：数据盘点、主体账户、资金规模、重点链路、异常专题、关系网络、资产产品、现金/境外、补证清单和报告候选状态。
5. **新增/强化 skill**：必须落实 `pair-amount-investigation`；新增 `delivery-qc` 或把 `analysis-critique` 扩展到等价能力；将 `relationship-network-analysis`、`asset-product-analysis` 作为 `investigation-lab` 一等专题，若高频则拆独立 focused skill。
6. **重写用户可见语言 policy**：把工程标题替换为经侦材料标题，例如“金额核验意见”“重点交易明细”“异常特征研判”“资金流向及断点”“需补调材料”。禁止用“统计范围不同不能相加”这种孤立技术句收尾，必须说明业务含义和下一步。
   - `簇/cluster/focus_cluster/outside-cluster` 只能作为历史字段、算法变量或内部审计对象存在，不得进入模型可见事实摘录、用户答案、报告、图表标题或 skill 默认正文。经侦表达统一改为：`相关账户（列明账号）`、`交易集合`、`集中交易组`、`主要集中转账`、`集中交易以外逐笔往来`、`时间集中`、`金额集中`、`可疑线索集合`。
   - 若为了兼容历史字段必须读取 `*_cluster` 数据，compiler 必须在输出前转换；测试必须同时覆盖翻译和泄漏阻断，不能只依赖人工写作规避。
7. **测试从词汇扫描升级为材料审查**：新增或扩展 `investigative_spine_contract`、`case_memo_plan_contract`、`evidence_maturity_language_contract`、`delivery_qc_contract`。断言最终回答是否包含事实表、解释、异常、侦查动作，是否足以让办案人员继续使用。

本节不要求在普通短问中强行生成长报告。正确做法是按用户意图和风险分层：低风险事实可以短答，但涉及金额 claim、主体画像、资金流向、全案、图谱、报告时，必须至少给出 mini-spine；用户明确要求“一句话”时也要保留必要的金额、期间和一句边界。

#### 0.0.8 2026-06-11 Analytix 软件工作流智能层 P0：不模板化，理解软件全过程

本节来自对两个无插件旧线程的复盘。无插件 Codex 最终能完成复杂任务，不是因为有固定模板，而是因为它会随着用户追问在 Analytix 工作流中切换：案件数据盘点、清洗表字段核对、金额重算、图谱绘制、报告补写、附件导出、全文核算、术语修正。插件的存在意义不是把这些步骤固定成机械链路，而是让 Codex 默认更懂 Analytix 软件和公安经侦资金分析。

旧线程行为模式必须产品化，但不能把旧案件事实写入 production reference：

1. **完整报告模式**：用户要完整资金分析报告时，Agent 应理解 Analytix 数据分析功能树，完成数据体量、时间跨度、导入/清洗/analysis 覆盖、主体/账户/对手方盘点、Top、异常特征、链路、续调清单和报告材料化。
2. **清洗明细模式**：用户要某人、某卡、某期间明细时，Agent 应回到清洗交易明细和账户维表，按 Analytix 清洗导出口径处理字段、别名、现金标识、摘要、对手缺失、重复和空对手，不把研判字段冒充原始清洗导出。
3. **链路深挖模式**：用户说继续追、钱从哪里来到哪里去、画图时，Agent 应围绕已核实资金边、未调取端点、下一跳和补证材料推进，而不是重新问用户要步骤。
4. **侦查假设模式**：用户围绕项目款、主材劳务、关联公司、亲属/关联人、资产端、现金/理财/证券等方向推进时，Agent 应形成可验证假设，区分已闭合链条、强支持线索、扩展线索和暂不能认定事项。
5. **材料复核模式**：用户要求金额必须真实正确、报告继续完善时，Agent 应逐项重算报告金额、明细、图块、术语和可读性，发现展示口径易误读就修正文档。

施工合同：

- 新增 `analytix_workflow_context` 或等价共享 reference：描述案件中心、当前案件同步、清洗/analysis 表、导出字段、图谱/报告/附件、工作目录和多轮状态如何协同。它是 Codex 理解 Analytix 软件的地图，不是用户可见说明书。
- `index` / root skill 必须先判断用户是在要数字、明细、研判、图、附件、报告、继续上一轮，还是修改已有材料；不得全部转成 `funds_investigate`。
- `case-context` 必须把 current case project、清洗表、analysis 索引、导出能力、报告目录、附件目录、图谱能力作为工作流上下文提供给 focused owner。
- `case-workbench` 必须支持真实的“按 Analytix 清洗表导出口径生成明细/附件”：显式导出走 `export_cleaned_case_data`，要求用户确认、当前案件边界、后端案件 `exports` 目录和 CSV/XLSX 读取检查；不得把所有 SQL 自定义分析都压成开发诊断，也不得把排队中、失败或未检查文件写成已交付。
- `visual-evidence` 必须支持在可用 Analytix/Codex 交付面上对 Mermaid、PNG/JPG、报告插图和证据表格做质量检查；无交付面时写明缺口，不得伪称已生成。
- `report-builder` 必须支持“继续完善既有报告”“按现有报告框架补写”“全文金额核算”“术语口吻修正”，而不是每次重建模板。
- `delivery-qc` 必须检查是否模板化、是否答非所问、是否理解用户正在推进的 Analytix 工作流。

新增验收：

- `analytix_workflow_context_contract`：断言当前案件、清洗/analysis scope、导出能力、报告/附件目录、多轮上下文被正确传给 focused owner。
- `non_template_output_contract`：断言同类问题在不同任务意图下不会输出同一套固定段落；正文必须随证据和用户目标变化。
- `cleaned_export_contract`：断言用户要求清洗明细/原始清洗字段时，必须创建或继续真实导出任务；输出字段、sheet 和口径与 Analytix 清洗导出一致且不混入研判列；生成文件须位于 Analytix-owned 目录并经读取检查后才能宣称交付。
- `report_continuation_contract`：断言继续完善报告时保留既有结构，局部补写、复核金额、更新图谱和术语，不重建短报告替代完整材料。
- `visual_artifact_contract`：断言 Mermaid/PNG/JPG/表格/附件只有在可用交付面真实生成并经渲染或读取检查后才能宣称交付，不用聊天图描述冒充交付。

本节的失败信号：用户明明在推进既有案件材料，插件却只答一个事实卡；用户要清洗明细，插件输出研判摘要；用户要图，插件只给 Mermaid 文本且不检查；用户要报告续写，插件重开模板；用户要深挖，插件只说“需复核”但不给查什么、调什么、怎么闭合。

#### 0.0.9 2026-06-14 真实线程 JSON 污染治理 P0：模型只看事实摘录，不看 support envelope

本节记录本轮真实 `analytixagent` rollout 复盘得到的新增 P0。旧线程只作失败背景；当前发布判断必须以本轮定位到的真实 Analytix-owned runtime rollout、当前插件源码和当前闭环产物为准。真实姓名、金额、thread/deeplink、JSONL 路径和逐轮量化指标只写入 `functional-closure/<timestamp>/`，不得写入 production reference、skill 默认上下文或 Hub package 用户材料。

复盘结论：

- 最终中文回答可以逐步变得专业，但如果 `function_call_output` 仍把大段 JSON / 转义 JSON / support envelope / 内部字段送给底座模型，仍判发布失败。成熟插件不会靠模型阅读大 JSON 归纳答案，而是把 source、validation、artifact 和 support 留在隔离层，只把读者可用的事实摘录交给模型。
- 真实线程里可见 MCP result 与模型可见 tool output 双重携带 `support_only`、`final_answer_owned_by_focused_skill`、`case_id`、`answer_card_complete`、`query_guidance`、`artifact_provenance` 等内部字段；这会诱导模型复述工具说明、口径标签或低质材料。
- 真实线程还暴露出 `exec_command cat .../SKILL.md`、列本地案件目录等行为。读取 focused skill 说明属于运行时 skill 机制，不应在真实办案问题里通过 shell 把完整 `SKILL.md` 输出塞给模型；当前案件来源也不得从本地目录猜测。
- 因此“最终回答看起来像材料”不能单独作为通过证据；必须同时证明模型可见工具输出已经从 JSON/support 文本降为短事实摘录，且内部字段、raw rows、workflow、case_id、debug label 不再外泄。

治理合同：

1. **普通模型可见工具输出必须非 JSON**：`agent-output-compiler`、`card-renderer`、frontdoor/card/runtime 默认返回 `案件事实摘录`、事实表或短边界说明；不得返回 `JSON.stringify(support envelope)`、raw rows、raw graph、workflow、case_id、delivery_state、answer_card_complete、supported seed、direct/candidate 或 answer_draft。
2. **support layer 只留 `_meta` / ledger / artifact / audit**：完整 payload、raw rows、SQL/notebook、graph detail、debug、artifact provenance、query guidance、known risks 只能保存在本地审计、`_meta` 或 artifact；普通 tool content 只携带最小事实、金额/笔数/账户/时间窗、主要边界和 focused owner 可用的证据线索。
3. **debug payload 双门控**：只有显式 `include_debug=true` 且 Analytix-owned 受控环境变量允许时，才可返回 structured/debug 内容；默认用户线程、普通 MCP 调用和真实 UI 复测不得暴露 debug JSON。
4. **卡片不写最终答案草稿**：card renderer 不输出可被模型直接复述的最终中文研判、固定小标题、完整支撑合同或“必须保留”文案；它只给事实摘录、验证状态和缺口，最终成稿由 focused owner 完成。
5. **真实线程审计成为发布门**：新增或使用等价的 `agent-thread-audit.mjs`，从 Analytix-owned runtime 定位 rollout JSONL，统计 tool calls、MCP payload bytes、模型可见 payload bytes、JSON/转义 JSON 次数、重复 JSON、内部字段泄漏、shell 本地猜案、最终回答材料质量和失败归因。
6. **质量判断双条件**：一个真实线程只有同时满足“最终回答像经侦材料”和“模型可见工具输出无 JSON/support 污染”才算通过；若最终回答好但 tool output 仍大 JSON，标记为 runtime/output compiler blocker。
7. **runtime cache 身份必须一致**：workspace 源码修复后，若 Analytix-owned runtime 仍加载旧版本插件，真实 UI 复测只能作为旧版本失败样本；不得用旧 runtime 结果证明当前源码已经闭环。

最小验证：

```bash
node plugins/analytix-fund-analysis/scripts/agent-thread-audit.mjs --recent 5
node plugins/analytix-fund-analysis/scripts/pair-amount-contract-smoke.mjs
node plugins/analytix-fund-analysis/scripts/user-visible-language-smoke.mjs
node plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs --profile b1 --forbid-user-language-leakage --json
```

若 backend、case-project source、runtime cache 或 Hub 安装阻塞真实 UI 复测，必须把阻塞写入 `manual-ui-notes.md`、`runtime-cache-sync.json`、`agent-thread-audit.json` 和 `release-identity.json`，不能把 CLI/direct MCP pass 冒充真实线程闭环。

#### 0.0.10 2026-06-14 真实追下游过度调用治理 P0：fundgraph 必须给可成稿事实包

本节记录 0.15.111 发布后同轮真实 `analytixagent` post-publish 复测暴露的新 P0：模型可见输出已经不再是大 JSON，但资金续查题如果只看到“图谱合同 / 边数 / 金额列表”，仍会自行扩展到重复 `rank`、`trace`、`pair`、`sql` 查询来拼出经侦材料。工具调用少不是唯一质量目标，但同一续查问题在没有新假设或新证据增量时反复查同一对象，说明 source helper 没有像 Data Analytics 那样一次交付足够事实。

治理合同：

1. **fundgraph / trace 输出必须可直接成稿**：`build_fund_flow_graph` 和后续 trace 类工具给模型的普通文本必须包含上游已支持边、金额/笔数/集中日、续查对象后续 Top 出账、可解释交易边、范围统计、不能认定事项和补证方向；不能只给 graph contract、edge count、rank number 或金额列表。
2. **聚合 facts 不靠模型再查**：若后端已经有 `source_to_via_summary`、`via_scope_stats`、`top_outflows`、supported edges 等结构化事实，MCP 层必须压成中文 fact pack；不得让模型再逐个 pair/sql/rank 验证同一组对象。
3. **真实线程成本纳入失败归因**：`agent-thread-audit` 和 `agent-ui-ab-run` 证据要同时记录 tool calls、tool_result_text_chars、model-visible bytes、over_budget 和重复同意图调用。最终回答好但成本异常，仍要归因到具体工具输出/skill/MCP，而不是写“模型发挥不稳”。
4. **修复方式是 source envelope 变好，不是加长 prompt**：通过 `fund_flow_fact_pack`、agent-readable renderer 和 runtime cache 发布包修复事实承载；prompt 只能作为补充边界，不可替代事实包。

最小验证：

```bash
node plugins/analytix-fund-analysis/scripts/agent-ui-ab-run.mjs --spawn-app-server --spawn-backend --tasks zhangjinzhi_continuation --modes full_plugin --max-new-runs 1 --json
```

验收口径：真实续查题必须在模型可见工具输出中看到中文事实表；工具调用应降到“首个 fundgraph + 必要一次续查核验”量级，不能重复扩展同一事实链；最终回答必须说明确定性边、统计窗口、异常线索、不能认定事项和补证方向。

### 0.1 失败事实

`analytixagent` 内置 Codex 在启用 `Analytix 涉案资金研判` 插件后，真实案件资金研判任务出现失败：用户要求围绕多个主体及其关联人、关联公司做资金梳理、资金关联通道和资金追踪，但插件输出偏向说明、门禁模板或无关内容，没有形成可交付研判结果。

该失败不能归结为用户提示词不清、模型偶发或单一 smoke 未覆盖。它说明插件实现与蓝本存在偏离，尤其可能出现在：

- 生产 runtime 混入固定 golden 案件样例。
- 追踪类任务被误路由到报告门禁。
- 当前案件 UI 选择没有稳定同步到 current case-project binding。
- Answer Card / Investigation Lab Card 没有把事实、线索、边界和下一步编译成用户可见结果。
- `check-health` 或固定 golden 分数被误当作真实任务质量证明。
- 默认只暴露少量前门工具，导致 Codex 看不见 rank/profile/trace/hypothesis/quality 等语义事实工具。
- `funds_investigate` 被实现成总指挥或固定链路，压制 Codex 自主选择工具、提出假设、改口径重算和继续追一层。
- 把来源/验证/交付边界误执行成工具禁令，导致 SQL/notebook/Python 等 Data Analytics 允许的可复核分析手段被压制，语义工具缺能力时只能降级说明。

2026-06-03 对暂停线程 `019e8da1-175c-7d22-b760-57842aa6ec56` 的复盘结论：

- 该线程已做过若干有价值修复，包括生产固定样例清理、输出污染阻断、Answer/Lab Card 整改、replay task/eval registry、当前案件同步证据。
- 但该线程仍沿用“蓝皮书对齐 + 前门总控 + diagnostic/report gate”路线，`funds_investigate` 仍偏总 commander，replay/eval 仍容易证明旧链路而非证明 Codex 更会想。
- 该线程还触碰了 `apps/desktop/src/main/agent-runtime-client.js` 及 desktop tests，超出本插件蓝本施工边界。新一轮施工必须先冻结和分类这些改动，除非用户明确要求，不继续扩大 desktop runtime 修改。
- 因此新一轮不是继续堆 gate、堆 JSON、堆 frontdoor，而是把插件改成 Codex 的可审计事实工具箱、案件图谱感知层和受控 Case Workbench。

### 0.2 本轮总目标

本轮执行目标是把插件重新拉回“让 Codex 更会想”的认知增强主线，但实现方式必须以 Data Analytics 的 source-backed 产品骨架为母版，而不是继续加固旧前门：

```text
轻量案件 source preflight
-> Codex 自主选择 focused skill 与最小充分事实路径
-> source helper / casegraph / fundgraph / Workbench / 图表 / 报告按需组合
-> live/source-backed verification
-> validation state / data quality / critique
-> 经侦风格 delivery contract
```

最终标准不是“测试能过”，而是：

- 插件生产路径不依赖任何固定案件样例。
- 用户真实侦查意图能由 Codex 自主映射到 scope、rank、profile、trace、hypothesis、quality、validation 等语义事实工具。
- 语义工具不足时不会拒答或乱答，而是像 Data Analytics 一样切换到可复核 SQL/Python/notebook/workbench 或明确 source blocker。
- 当前案件能从 analytix 案件项目 UI 选择同步到 Analytix-owned runtime、source helper、workbench 和 MCP 承载层。
- 插件能输出研判结果，不输出说明书式内容。
- 插件在真实案件任务中形成 source-backed、validation-backed、delivery-backed 的功能闭环。
- 蓝本、reference、runtime、doctor、功能测试、真机测试彼此一致。

### 0.3 本轮非目标与发布边界

- 发布、封包、admin-console 上传只属于 15.6 授权发布阶段；常规蓝本修补阶段不得把未验收产物当成已发布版本。
- 不改系统 Codex。
- 不改系统 Codex 的全局 home/config/cache 目录。
- 不把插件做成独立业务系统。
- 不继续扩大 desktop runtime / `analytixagent` client 改造，除非当前案件同步已被证明确实只能在该边界修复且用户明确同意。
- 不为了过测试降低证据门禁。
- 不靠长 prompt、大 JSON 或工具扩散制造增强假象。
- 不把 `funds_investigate` 做成唯一前门或总指挥。
- 不把 rank/profile/trace/hypothesis/quality 等高价值语义工具藏到 Codex 默认不可见。

### 0.4 成熟插件行为标准

本轮整改不能只把失败链路修到“能调用工具”。插件必须呈现成熟商业插件的行为：让 `analytixagent` 内置 Codex 在案件资金研判中更会理解意图、更会组织工作流、更会质疑事实口径、更会控制输出风险，而不是把 Codex 限制成只能读取事实卡的一问一答工具。

成熟插件行为必须同时满足：

- 短入口：root skill 只负责触发、边界和渐进披露，不把全部领域知识塞进默认上下文。
- 能力可发现：插件页、index/root skill、focused skills、command metadata、progressive resources 能让模型和开发者发现资金穿透、对象画像、全案分析、开放深挖、报告复核等能力；单 root skill 只可作为兼容入口，不能作为商业成熟完成态。
- 语义事实工具箱：生产默认工具面应暴露 scope、rank、profile、trace、casegraph、hypothesis、quality、validation 等高价值工具；低层 raw/debug/write/export/report-heavy 工具才受条件发现控制。
- 工作流增强：复杂任务能自动形成 scope、lane、预算、证据门禁、下一步追查队列和收口边界，但普通问数、排名、画像、追一层不能被流程化压住。
- 反模式治理：普通问答、排行、图谱、Lab、报告都要进入 anti-pattern / critique pass，不只在报告末端做 claim review。
- 自主研判保留：约束数据访问和事实表达，不压制 Codex 提出假设、追问口径、设计补证路径和组织材料。
- 本地可回放：所有质量结论都能通过 doctor、功能测试、trace artifact 和人工真机记录回放，不接第三方遥测。
- 隔离可信：不改系统 Codex、不污染系统 Codex 的全局 home/config/cache 目录、不把 Analytix-owned runtime cache 当成系统插件状态。

成熟插件失败信号：

- 插件输出主要是能力说明、门禁说明或“请提供 case_id”。
- 插件把追踪任务压成报告门禁，或把正式报告门禁扩散到普通研判。
- 插件把异常发现、假设提出和可执行追查压缩成固定查询或固定报告链路。
- 插件为了降低幻觉而禁止开放追查，只保留保守事实查询。
- 插件把高价值语义工具隐藏，只留 `funds_investigate`、报告复核和续查复核，导致回答固定链路化。
- 插件为了显得强大而暴露低层工具、长 JSON、raw rows 或内部调试资源。
- 插件页只显示一个 skill 时，没有其他能力发现入口支撑 registry 中声明的命令和能力族。

### 0.5 方案假设与论证

| 方案 | 假设 | 优势 | 失败风险 | 结论 |
| --- | --- | --- | --- | --- |
| A. 继续蓝本旧路线：统一前门 + Plan DAG + report gate | 只要把固定样例和路由修好，前门总控就能稳定。 | 工具调用少，门禁强，容易做 smoke。 | Codex 看不见语义工具，开放深挖和改口径重算仍被收口边界压住；回答继续机械。 | 否决，最多保留 formal report 的计划化链路。 |
| B. 暴露全部 MCP 工具 | Codex 自由选择越多越强。 | 自由度最大，短期容易绕过前门失败。 | raw rows/debug/heavy 工具扩散，成本飙升，口径漂移，模型可能重新裸拼事实。 | 否决，低层工具必须受控。 |
| C. 只写更多 `SKILL.md` / command instruction | 学 impeccable/superpowers，用更好指令引导模型。 | 工程量小，能改善表达和反模式意识。 | 没有事实工具可见性和 casegraph 支撑时，仍是 prompt discipline，无法稳定完成真实案件功能闭环。 | 作为辅助手段，不作为主路线。 |
| D. 语义事实工具箱 + casegraph 感知 + Controlled Case Workbench + 后置 critique | 只锁事实来源，不锁思考路径；把工具变成 Codex 的认知器官，并给语义工具缺口一个受控通用分析兜底。 | Codex 可自主选择 rank/profile/trace/hypothesis/quality；明确自定义口径可进入当前案件、只读、清洗表/分析索引、限行、可回放的 workbench；事实仍由 MCP/ledger/verifier 约束。 | 需要重写 discovery policy、registry、tool descriptions、eval、输出编译和 workbench 安全门禁，工程量中等偏高。 | 采用。 |

最终路线：

```text
默认可见语义事实工具箱
-> funds_investigate 降级为 navigator
-> casegraph / scope / coverage 先感知
-> Codex 自主选择 rank/profile/trace/hypothesis/quality
-> 语义工具不足时进入 Controlled Case Workbench
-> Context Compiler 压缩事实
-> Anti-pattern / Critique Pass 后置
-> report lane 才进入 Claim Verifier 强门禁
```

### 0.6 最终产品架构选择

本轮之后，Analytix 的蓝本不再停留于“单 skill + 多命令 + 多工具”。最终商业形态采用：

```text
Data Analytics 式多 skill 产品架构
+ Impeccable 式领域词汇、反模式和 critique
+ CodeGraph 式 casegraph/fundgraph 上下文
+ Superpowers 式复杂全案计划/复核
+ oh-my-codex 式 doctor/runtime 状态治理
```

执行含义：

- Data Analytics 是主骨架：`index` 总入口、`case-context` 预检、多个 focused workflow skill、少量 delivery/review sub-skill。
- Data Analytics 的 `visualize-data`、`build-dashboard`、`spreadsheets` 精髓落到 `visual-evidence`：表格、图表、看板和附件工作包必须有 owner、workflow、质量门和证据边界；不得依赖 Data Analytics runtime 或把商业 KPI 语义搬进经侦案件。
- Impeccable 是质控层：`/analytix` 命令族、经侦词汇、证据状态、deterministic anti-pattern、LLM critique pass。
- CodeGraph 是上下文层：casegraph/fundgraph/scope inventory 让 Codex 更会看案件，不替 Codex 决策。
- Superpowers 只进入复杂全案、正式报告、重大 QA，不覆盖普通问数。
- oh-my-codex 只借鉴 doctor、状态、ledger/checkpoint，不改系统 Codex，不写全局 hooks/config/cache。

多 skill 的目标不是增加数量，而是解决单 skill 总入口导致的语义塌缩：用户说“某卡/某人资金研判”时必须进入对象画像，用户说“全案分析”时必须进入全案分析树，用户说“生成报告”才进入报告和 claim gate。

Data Analytics 借鉴落点必须按下表施工，不能把商业分析插件的语义照搬到经侦案件：

| Data Analytics 能力 | Analytix 落点 | 施工要求 |
| --- | --- | --- |
| `index` | `index` | 只做路由和能力介绍，不执行事实研判。 |
| `user-context` / context gathering | `case-context` | 只预检当前案件、清洗表、scope、casegraph/fundgraph、运行状态，不做跨案记忆。 |
| data quality / validation | `data-quality` / `claim-review` | 清洗覆盖、重复/换卡、空户名、缺失对手、金额单位、报告 claim 都要有质量门。 |
| product/business analysis / diagnostics | `full-case-analysis` / `investigation-lab` / `analysis-critique` | 学“问题分解、异常解释、反证、下一步建议”，不学商业 KPI 语义。 |
| visualization / dashboard / spreadsheet | `visual-evidence` / `evidence-request` | 表格、Top20 图、特征矩阵、看板卡、附件工作簿必须证据绑定，不能新增事实或法律结论。 |
| report building | `report-builder` | 报告先有事实树、视觉证据和 claim review，再进入公安经侦公文口吻。 |
| notebook / semantic-layer | `case-workbench`：当前案件、只读、清洗表/分析索引、限行、可回放的 SQL/notebook/custom口径兜底；semantic-layer 思路用于 casegraph/scope map。 | 不暴露跨案语义层，不把未验证计算、样本行、局部预览或弱来源写成普通研判事实。 |

#### 0.6.1 Data Analytics 对齐后的最终施工合同

本轮复核后，Data Analytics 对 Analytix 的最重要启发不是“可以写 SQL”，而是完整的 source-backed workflow：

| Data Analytics 做法 | Analytix 必须落地 | 阻断信号 |
| --- | --- | --- |
| semantic layer 只是地图，结论仍需 live source read | `case-context` / `get_case_scope_map` 只做案件事实地图；金额、笔数、Top、路径仍要 source-backed helper、controlled workbench 或 evidence ledger 返回事实。 | scope map、dashboard metadata 或样本行被写成报告级事实。 |
| source access guardrail | 必需事实源缺失时停止报告级结论；可选 enrich 缺失时继续但标注 gap。 | 缺开户/联系/住址/IP/MAC/调单反馈就写成已搜索全量，或把字段缺失写成无关联。 |
| notebook 是交付物，不是草稿堆 | `case-workbench` 必须记录目的、源范围、SQL/notebook 单元、行数、验证状态和 handoff。 | notebook 不可回放、无 scope、无验证状态，或暴露本地路径/raw rows。 |
| visual/report/dashboard 有 owner 和 QA | `visual-evidence` 负责表格/图表/看板/附件；`report-builder` 负责正式材料；两者都不能由 chat summary 替代。 | 表格/图表没有 scope/unit/time window/metric/direction/evidence status，或报告没有 claim review。 |
| capability gap 可以先用受控分析回答，再产品化 | 语义工具不足时先换语义路径；仍不足且任务明确时进入 Case Workbench；可复用缺口登记为 MCP 产品化候选。 | 把“以后沉淀 MCP”变成当前任务拒答理由，或把弱来源/未验证计算当成 source-backed 事实。 |

因此，“沉淀为新的 source helper / backend / MCP 能力”是能力演进闭环，不是每次自定义口径的前置门禁。当前案件能在受控专项核算中回答的，应给出受控结果、证据边界和下一步；只有受控专项核算不可用、权限/清洗表/安全策略阻断或结果不能支撑报告级研判结论时，才输出专项核算缺口说明。

#### 0.6.2 版本升级、部署封包与能力缺口生命周期

Analytix 插件的升级模型必须和 Data Analytics 式官方插件更新一样清晰，但运行边界属于 Analytix：

- 新版 Analytix 可以内置新版 `analytixagent` 和新版 `analytix-fund-analysis` 插件；已安装产品也可以通过 Analytix Hub / admin-console 插件更新进入 Analytix-owned runtime。
- `sync-runtime-cache.mjs` 只是本地同版本 remount 验证，不是发布、安装、升级或客户更新路径。
- 插件不能在运行中自动改 backend、自动新增 MCP 工具、自动写安装态、自动升级自己。能力缺口只能先生成 Gap Card / backlog，再由工程实现 source helper / backend / MCP、schema、skill、eval、doctor 和版本发布。
- 发布或封包前必须证明 manifest version、MCP server version、Capability Registry、tool schema、required skill mount、Hub package hash、installed runtime cache 和 runtime health 同身份一致。
- 兼容性必须按“旧版本 runtime 不认识新工具时返回能力缺口，新版本工具不破坏旧语义工具”的方式设计；不能让新版插件在旧 backend 上伪造 workbench 结果。

下一轮 `/goal` 若进入 source helper / workbench / MCP 施工，P0 不是继续改蓝本，而是实现并验证：`run_case_sql` / `create_case_notebook` 的真实只读执行、workbench 审计 artifact、semantic-tool fallback、真机功能闭环和版本/Hub package 身份一致性。

#### 0.6.3 Data Analytics 等价边界验收

从本节开始，后续施工线程不得再给 Analytix 自创 Data Analytics 没有的工具禁令。验收时只检查 Data Analytics 已经证明有效的机制：

| Data Analytics 机制 | Analytix 等价落地 | 不合格信号 |
| --- | --- | --- |
| preflight envelope | `case-context` / current case project / scope / cleaned-table coverage / casegraph/fundgraph 先给任务上下文、可用来源、语义地图、目标交付物和最终回答义务。 | 没有当前案件、scope、coverage 或交付目标就输出确定结论，或把 preflight 当硬路由。 |
| semantic layer as map | casegraph/fundgraph/scope map 只是定位事实、候选路径、表字段和口径的地图。 | 把图谱、scope map、dashboard metadata、样本行或语义说明当最终事实。 |
| source-of-truth selection | 每个金额、笔数、账户、主体、对手方、路径、图表和报告 claim 都要说明由哪个清洗表、analysis index、MCP 事实卡或受控 workbench 结果控制。 | 来源冲突时不说明优先级；用弱来源、记忆、样例或工具说明替代控制来源。 |
| source guardrail | 必需来源缺失时停止报告级 claim；可选 enrichment 缺失时继续但标注 gap 和影响。 | 把缺字段写成不存在；把缺失来源解释成案件事实；把可选缺口当成全任务失败。 |
| live/source-backed verification | 数据结论必须实际读取、查询、计算或复核；语义层和已有上下文只能指路。普通任务优先最小充分 source-backed 路径，语义 helper 不足且任务明确时允许 `case-workbench` SQL/Python/notebook。 | 凭记忆、固定样例、旧 evidence、工具介绍、schema 名字或未执行 SQL 直接作答。 |
| SQL / Python / notebook allowed when useful | Data Analytics 允许的分析手段 Analytix 不得自创工具封锁口径；只要求当前案件、清洗/analysis scope、只读、限行、purpose、validation 和 evidence boundary。 | 把 DuckDB/SQL/Python/notebook/shell 等可复核分析手段写成不可用，或因缺新 MCP 直接拒绝明确可分析任务。 |
| data quality checks | `data-quality` / `case-context` / `case-workbench` 检查粒度、时间窗、缺失、重复、join 风险、异常值、freshness、清洗覆盖和口径漂移。 | 不查 grain/窗口/重复/覆盖就写“全量”“全部”“无异常”；把 bounded preview 当全案统计。 |
| validation state | `data-quality`、`claim-review`、`analysis-critique` 明确已验证、部分验证、线索/可能、needs_review、blocked、unavailable。 | 只追求分数，不复核真实 claim、SQL/计算口径、视觉误导或交付缺口；把线索写成已查明事实。 |
| delivery contract | `visual-evidence`、`report-builder`、notebook/workbench artifact 只有在可用交付面真实生成并检查后才可宣称交付；否则必须明确 blocker；聊天摘要不能替代交付物。 | chat summary 冒充报告、表格、图表、看板、notebook、附件包或证据包。 |
| render/execute QA | 报告要打开或渲染检查，notebook/workbench 要记录执行状态，图表/表格要在最终容器检查。 | “已生成/已交付”但没有产物、没有执行状态、没有截图/渲染/读取检查或没有 blocker。 |
| audience language | 普通用户输出分析结论、口径边界和下一步；内部状态、评测器、诊断标签、大 JSON、工具实现细节留在 audit/evidence。 | 把蓝本、/goal、doctor、内部评测标签、report gate、内部 id、SQL dump、raw rows 或大 JSON 输出给办案用户。 |
| index / navigator not commander | root/index/funds_investigate 只做能力发现、意图归类、focused skill 选择和模糊入口导航。 | index 或 funds_investigate 重新垄断 rank/profile/trace/lab/report/workbench，变成机械总指挥。 |
| focused workflow ownership | 对象画像、全案分析、资金追踪、开放研判、报告、visual evidence、workbench 等由对应 focused skill 拥有 workflow 和完成门。 | root skill 长文替代 focused skill；每个任务只套同一固定链路或同一工具预算。 |
| report completion gate | 进入报告路线后必须完成报告形态、claim 复核、证据/图表/附件交付或明确 blocker。 | 用普通 inline 答复冒充正式报告；报告无 claim review、无渲染/读取检查或无附件边界。 |
| scoped state / memory | 只保存 Analytix-owned、本案内 source、scope、artifact、journal、semantic-map 指针和运行状态。 | 保存跨案事实记忆、全局偏好、系统 Codex 状态，或跨案复用 artifact/ledger/hypothesis。 |

因此，本轮测试不能再围绕“有没有禁用某个本地工具”或“分数是否好看”展开；必须围绕“是否有来源信封、是否 live/source-backed、是否验证、是否真实交付、是否按用户意图完成经侦资金分析”展开。

功能闭环产物只接受当前候选版本、当前 HEAD、当前 runtime identity 下生成的 `output/analytix-fund-analysis/functional-closure/` 产物。插件源码目录下不再保留历史 `evidence/` 对比实验产物；旧产物只能作为人工背景，不得进入 release guard、doctor 或 Hub package 判断，避免旧口径、旧禁令和旧 golden 反向污染新版本验收。

以下产物不得作为功能闭环依据：旧版本/旧 HEAD/旧 runtime cache；旧工具封锁口径；只看平均分或脚本通过；没有 runtime cache preflight、backend、current case project 和 release identity；没有真实 analytixagent 前门功能任务，只跑 CLI/direct MCP；输出 JSON 很完整但真实用户问题仍答错、答浅、答成说明书或丢失侦查意图。

#### 0.6.4 `case_source_envelope` 统一来源信封

Data Analytics 的 preflight envelope 不是一个总指挥，而是后续分析选择来源、验证和交付状态的共同事实底座。Analytix 等价物命名为 `case_source_envelope`，它不替 Codex 决策、不阻止 SQL/Python/notebook，而是让 Codex 在做案件资金事实前知道“当前能信什么、还缺什么、交付到了哪一步”。

`case_source_envelope` 至少包含：

- `case_identity`：caseProjectCaseId、case_id、case_name、active/source/status。
- `source_of_truth`：交易事实以清洗明细和 approved analysis index 为准；开户/户名/证件/联系方式/住址以清洗开户信息和主体维表为准；casegraph/fundgraph/scope map 只作地图和候选路径。
- `source_scope`：清洗表、analysis 索引、可见表/视图策略、不可事实化来源、行数/样本边界。
- `data_quality_state`：导入/清洗覆盖、时间跨度、方向/金额/户名/对手方字段缺口、重复/换卡/同事实候选、未索引来源。
- `metric_scope`：时间窗、方向、金额单位、成功/失败交易、去重/同事实口径、主体/账户/对手方粒度。
- `validation_state`：已验证、部分验证、needs_review、blocked、unavailable，以及影响的 claim 类型。
- `delivery_state`：inline answer、table/chart inventory、notebook/workbench artifact、report draft、appendix pack 是否真实生成。
- `gap_card`：缺失来源、缺失工具、未执行计算、未完成渲染、不能支撑的结论和下一步产品化目标。

施工要求：

- `case-context`、`data-quality`、`case-workbench`、`visual-evidence`、`report-builder`、`claim-review` 的输出和测试都要能映射到这个 envelope。
- `casegraph/fundgraph`、semantic map、bounded preview 只能写入 `source_scope` 或 `context_map`，不能升级为 source of truth。
- Workbench 成功时必须回填 `metric_scope`、`validation_state` 和 `delivery_state`；失败时只回填 `gap_card`，不能假装已计算。
- 报告、图表、附件和 notebook 任务必须检查 `delivery_state`；没有真实 artifact 或明确 blocker，不得写“已生成/已交付”。
- 功能测试只认可 envelope 支撑的分析质量，不认可旧产物目录、旧 golden、内部字段名、工具禁令背诵或追分式模板。

#### 0.6.5 Data Analytics 式性能与上下文施工合同

大改必须同时解决“机械总指挥”和“重验算耗 token”两个风险。施工线程不得把 source-backed verification 实现成每题全量扫描、每题 notebook、每题 claim review、每题对比实验。性能合同如下：

| 场景 | 默认预算 | 升级条件 | 不合格信号 |
| --- | --- | --- | --- |
| 低风险 Quick Fact / Top | 一个最小 source-backed 路径，必要时再补一个 validation。 | 工具返回 `needs_review`、金额争议、重复/换卡、holder/account 粒度不清。 | 为一个 Top 问题跑 full-case/report；或只因省工具而弱来源强答。 |
| Pair Amount / 金额质疑 | 先拿 scope + controlling source，再做 targeted 聚合/去重核验。 | rank 工具无法控制口径、存在同事实重复、卡号换卡、对手粒度歧义。 | 直接朗读排行金额；或为一个 pair 问题跑全案报告。 |
| 对象画像 / 全案分析 | 进入 focused workflow，按 mandatory card 分层取事实。 | 用户要求完整研判、全案、报告、附件或侦查深挖。 | 把“某卡/某人研判”降级成简单问数；或每个对象都调用无关工具。 |
| Visual / Report / Notebook | 必须有真实 artifact、render/execute QA 和 delivery_state。 | 用户要求图表、报告、附件、notebook、可回放计算。 | 聊天摘要冒充交付；或无产物却说已生成。 |
| 功能闭环 / 发布身份 | 只验证当前版本、当前 runtime、当前案件和真实交付状态是否一致。 | 候选版本、runtime identity、current case project、真实前门功能任务已确认。 | 施工阶段围绕分数反复跑，忽略真实错误、交付缺口或版本漂移。 |

实现要求：

- root/index/focused skill 默认只加载短入口和必要 reference；大字段、raw rows、SQL dump、trace、debug payload 进入 artifact、ledger 或 `_meta`，不进入普通模型上下文。
- `case_source_envelope` 必须可压缩：普通问答只带 source/metric/validation/delivery 摘要，报告和 workbench 才展开证据细节。
- doctor/功能测试要能检测过度调用、无效重链路、追分导向和假交付；但不得把“工具调用少”本身当质量成功。
- 后续新增 MCP 语义工具要优先降低重复查询和上下文成本，而不是增加模型必须阅读的 JSON。

### 0.7 任务分型施工合同

施工必须把任务分型落到 `SKILL.md`、focused skill、command metadata、capability registry、tool discovery、功能测试和真机功能验收中。

| 任务档位 | 必须路由 | 最小验收 |
| --- | --- | --- |
| Quick Fact | `quick-fact`，rank/profile/trace 中一个最小充分 source-backed 路径。 | 低风险事实一轮给事实、口径、边界；不扫全案、不进报告。若触发重复/换卡/账户归属/join/时间窗风险，升级一次 targeted validation。 |
| Pair Amount / Amount Challenge | `pair-amount-investigation` + `data-quality`，必要时 `case-workbench`。 | “A 转给 B 多少钱”“两个金额范围哪个对”必须核对 source-of-truth、holder/account scope、counterparty grain、raw/effective/dedup 口径、时间窗、同事实/换卡风险；不得只用 `rank_counterparties` 或 `funds_investigate` 事实卡最终作答。 |
| Ranking | `quick-fact`，`rank_accounts/rank_holders/rank_counterparties`。 | Top20/排行有 metric、direction、time window、success filter、coverage；排行只能回答排行，不自动升级为报告级金额、路径或去重结论。 |
| Object Dossier | `account-dossier` 或 `subject-dossier`。 | 某卡/某人资金研判必须包含基本情况、开户/身份/联系电话/住址线索、账户结构、时间跨度、进出账金额笔数、Top 对手、异常特征、来源/去向、证据缺口、下一步；联系方式/住址只能作为线索，不能升级为控制或代持事实。 |
| Counterparty | `counterparty-analysis`。 | 共同对手、关联通道、缺失对手、账号型对手和补调优先级分层。 |
| Trace / Continuation | `fund-tracing`。 | 继续追一层、下一跳、来源/去向不受 `answer_card_complete` 压制；supported edge 与候选断点分离。 |
| Investigation Lab | `investigation-lab`。 | 假设队列、证据状态、forbidden-as-fact、下一步追查队列齐全。 |
| Full Case Analysis | `full-case-analysis`。 | 全案数据量、时间跨度、清洗覆盖、进出账金额笔数、按人/账户统计、Top20、可疑特征、资金链路、专题线索、续调清单。 |
| Report Builder | `report-builder`。 | 只有明确报告意图才写报告；先选择全案汇报材料、一人/一企一档、工程/职务犯罪/串标专题、资金去向追踪及续调清单、单线索核查等报告形态；报告 claim、附件、Mermaid 和公安经侦公文口吻必须复核。 |
| Evidence Request | `evidence-request`。 | 补调/续调/取证清单写清对象、事项、字段/时间范围、证明目的、优先级和边界；请求材料不是已取得证明。 |
| Analysis Critique | `analysis-critique`。 | 复核研判是否机械、漏掉经侦 lane、路由错误或证据边界弱，并建议下一步 focused skill。 |
| Visual Evidence | `visual-evidence`。 | 表格、图表、看板和附件工作包从已复核事实生成；每个表/图必须有 scope、unit、time window、metric、direction、evidence status；排行、矩阵和看板不得写成交易路径、控制关系或法律结论。 |
| Case Workbench | `case-workbench`。 | 只有明确 SQL/notebook/custom口径/可回放计算，且现有语义工具不足时才进入；必须当前案件、只读、清洗表/分析索引、限行、purpose、可回放、验证状态清楚；不能把弱来源、`fc_*_raw`、DDL/DML、全量导出或未验证计算写成生产事实。 |
| Claim / QA | `claim-review` / `data-quality`。 | 金额、链路、Mermaid、法律敏感表述、附件清单有通过/阻断/降级。 |
| Passive | 无资金 skill。 | 非案件资金任务不调用 MCP。 |

失败即阻断：

- “某卡/某人资金研判”被当成普通事实题，只输出一个金额、Top 或工具摘要。
- “全案分析”只输出概览、计划或误入 report gate，没有跑全案分析树。
- “继续追一层/改口径重算/否定性搜索”被上一轮完成状态压住。
- focused skill 只是复制 root skill 长文，或只是 MCP tool 的外壳。

### 0.7.1 经侦领域能力施工合同

`economic-investigation-analysis.md` 是领域任务库，不是长 prompt。施工必须把其中的案类 typology lens、异常特征库、来源/去向追踪、negative search、金额质疑包和办案交付包落到 focused skill、metadata、registry、eval 和输出质检中。

阻断条件：

- typology 被写成法律定性，例如从银行流水直接认定诈骗、跑分、洗钱、受贿、虚开、侵占或非法获利。
- “资金来源”只支持下游去向，缺少 upstream source tracing 语义和输出边界。
- “异常特征跑一遍”只列工具或建议，没有 feature family、supporting facts、evidence status、downgrade reason、next proof。
- 办案交付只给聊天摘要，没有事实表、主体/账户/对手方表、特征表、流向表、补调清单、图表/看板/附件工作包或 claim QA。
- `data-quality` 被实现成导入/清洗执行器，而不是只读 coverage/audit/口径复核。

### 0.7.2 经侦清洗表事实层与完成包施工要求

当前案件分析必须默认理解为多层清洗表/分析索引事实工作，而不是只围绕资金明细表做金额查询。施工时必须把以下层级落到 `economic-investigation-analysis.md`、focused skills、metadata、capability registry、功能测试、输出 critique 和真机功能验收：

生产事实的 source-of-truth 优先来自 Analytix 数据清洗模块产出的 `fc_*_norm`、`analysis_*`，以及 backend / controlled workbench / MCP 承载的可审计编译事实卡。`fc_*_raw`、原始文件、raw rows 和未复核计算可以服务导入/清洗模块、开发诊断或专项质量排查，但不能被普通研判 skill、source helper 或 MCP 直接升级为 production claim。若现有语义工具不足以回答明确自定义口径，`case-workbench` 可以通过 Analytix-owned runtime 在当前案件、只读、清洗表/分析索引、限行、可回放边界内执行受控 SQL/notebook，并把可复用能力沉淀为新的语义工具候选。

| 事实层 | 施工要求 | 阻断信号 |
| --- | --- | --- |
| 清洗交易明细层 | `fc_transaction_norm`、`analysis_txn_detail_idx` 的金额、笔数、方向、时间、账号、对手方、摘要、交易边只能由确定性事实工具支撑。 | bounded rows 或样本行被汇总成全案 totals；回退 `fc_*_raw` 算事实。 |
| 交易环境/设备/渠道层 | 清洗交易索引中的 IP、MAC、柜员号、网点、地点、凭证/终端、商户名/商户号、支付渠道必须作为可选经侦线索层进入 scope、profile、lab、full-case 和 negative search。 | 同 IP/MAC、同柜员、同网点、同商户写成实际操作人、控制人、团伙、共同犯罪或同一物理人。 |
| 开户/账户登记层 | `fc_account_norm`、`fc_sub_account_norm`、`analysis_account_dim` 的户名、证件、开户行、账户类型、状态、开户/销户等作为身份与范围证据。 | 开户信息被写成实际控制、资金流、交易目的或法律身份。 |
| 人员/联系电话/住址层 | `fc_person_norm`、`fc_person_contact_norm`、`fc_person_address_norm` 的姓名、证件、电话、住址、单位、法定代表人作为身份/关联线索。 | 同电话、同住址、同单位写成控制、代持、共犯、资金流。 |
| 调单反馈与查控层 | `fc_task_success_norm`、`fc_task_fail_norm`、`fc_coercive_measure_norm` 必须用于覆盖缺口、失败反馈、冻结/扣划/查控线索。 | 失败反馈写成无账户；查控措施写成犯罪事实或非法所得。 |
| 主体/户名索引层 | 直接账户、候选账户、同名/别名、公司/个人、主体池必须分层。 | candidate-linked account 写成名下账户。 |
| 对手方索引层 | Top 对手、共同对手、缺失对手、账号型对手、支付/商户线索要有补调优先级。 | 共同对手写成确认关系，缺失对手写成隐藏身份。 |
| casegraph/关系层 | 作为上下文和假设来源，帮助 Codex 更会看案件。 | graph edge 被当作 supported transaction edge。 |
| 管线与口径层 | import/source coverage、cleaned、analysis index、report scope、重复/换卡候选、未进索引清洗表缺口要分开。 | 混用 scope、静默去重、候选重复直接扣减 totals。 |

三类核心完成包必须成为实现和验收的 P0：

- 单卡/单账户研判：开户/登记、账户状态与类型、交易覆盖、时间跨度、进出账金额笔数、Top 对手、账户角色、生活卡/中转/汇聚/现金/资产/理财/支付/境外线索、IP/MAC/柜员/网点/商户/渠道线索、来源/去向、证据缺口、下一步。
- 某人/某公司主体研判：身份与开户事实、联系电话/住址/单位/法定代表人关联线索、直接/候选账户边界、每账户角色、账户组合排行、主体进出账、Top 对手、共同通道、关联人/关联公司、IP/MAC/设备/商户/渠道重合、资产/现金/理财/境外线索、补调优先级。
- 全案分析：数据量、时间跨度、cleaned/index/report coverage、开户信息覆盖、人员/公司/账户/联系电话/住址/交易环境/IP/MAC/调单反馈/查控措施覆盖、按人/账户统计、每个重点主体名下账户、进出 Top20、账户角色分布、团伙/共同控制关联线索、现金同存同取、资产消费、投资理财、支付通道、商户平台、虚拟资产/OTC、境外/跨境、票税合同、工程/涉诉/工资报销/对公转私两层统计、正常成本与疑似输送分层、资金链路、否定性发现、补调清单和报告可用事实。

本节落地后，`某卡资金研判`、`某人资金研判`、`全案分析` 不允许退化成 quick fact、摘要或计划。eval 必须覆盖账户角色、开户信息、联系电话/住址、IP/MAC/交易环境、调单反馈/查控措施、生活卡正常性、中转/汇聚、团伙/共同控制关联线索、现金同存同取、资产理财、支付通道、商户平台、虚拟资产/OTC、境外线索、票税合同、两层统计、正常成本分层和全案完整包。

### 0.8 多 skill 施工边界

本轮若进入多 skill 落地，必须遵守：

- 先改蓝本、执行方案、registry/metadata/eval，再 scaffold skill；不能先堆目录再补语义。
- 保留旧 `analytix-fund-analysis` skill 作为兼容入口，直到新 focused skill 通过真机功能验收。
- 新 skill 目录只写短 `SKILL.md`、一层 references 链接和必要 `agents/openai.yaml`；不得复制整份蓝本。
- 共享硬边界、事实工具箱、证据规则和输出规则应抽为 shared reference，避免 10 多个 skill 文案漂移。
- `runtime-cache-contract.mjs`、doctor、plugin manifest、Hub/UI 展示必须能识别多 skill，并核准 root/skill references 同步。
- 多 skill 完成前，不允许把“UI 技能数量增加”当成质量证明；必须用 Object Dossier、Full Case、Trace、Report、Passive 等真实功能场景证明。

## 1. 文档与程序关系

### 1.1 三层基准

| 层级 | 文件 / 载体 | 作用 |
| --- | --- | --- |
| 系统蓝皮书 | `references/top-pluginization-plan.md` | 定义插件最终能力、边界、架构、质量、隔离、商业成熟标准。 |
| 执行方案 | `references/blueprint-execution-plan.md` | 定义本轮如何按蓝皮书完成审计、修复、测试、功能闭环和验收。 |
| `/goal` 提示词 | 本文件末尾 | 给 Codex 单线程执行的任务入口，不承载全部架构判断。 |

### 1.2 冲突处理

| 冲突 | 处理规则 |
| --- | --- |
| 程序与蓝皮书冲突 | 默认程序错，先按蓝皮书修程序。 |
| 蓝皮书与真实测试冲突 | 判断蓝皮书是否过度限制、表述不完整或遗漏真实链路；若蓝皮书错，修蓝皮书。 |
| 测试与蓝皮书冲突 | 测试不能为了分数牺牲蓝皮书；若测试断言过时或过窄，修测试。 |
| 文档之间冲突 | `top-pluginization-plan.md` 为最高系统基准；专项 reference 必须向其对齐。 |
| root reference 与 skill reference 冲突 | 必须同步，不能只改一份。 |

### 1.3 文件同步要求

每次修改本文档时，必须同步：

- `plugins/analytix-fund-analysis/references/blueprint-execution-plan.md`
- `plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-execution-plan.md`

并检查：

- `scripts/runtime-cache-contract.mjs` 的 `REFERENCE_FILES`。
- `mcp/progressive-resources.mjs` 是否暴露该 resource。
- `scripts/doctor.mjs` 是否需要把该 resource 纳入 required URI。
- `top-pluginization-plan.md` 是否需要纳入 reference 映射。

## 2. 唯一施工线程规则

### 2.1 为什么必须单线程

本轮整改涉及 runtime、路由、证据卡、测试、蓝皮书、skill reference、runtime cache 和真机测试。如果多个 Codex 线程并行执行，会出现：

- 一个线程清 production runtime，另一个线程又把固定样例写回去。
- 一个线程更新 root reference，另一个线程忘记同步 skill reference。
- 一个线程按 CLI smoke 调整，另一个线程按 UI 失败调整，导致标准冲突。
- 一个线程改测试放松断言，另一个线程误以为插件能力变强。
- 多线程互相覆盖未提交改动，无法回放根因。

因此本轮只允许一个主施工线程。只读审计线程也不建议开启；若必须开启，只能输出审计报告，不得改文件。

### 2.2 施工线程输入

主线程开始前必须读取：

- `references/top-pluginization-plan.md`
- `references/blueprint-execution-plan.md`
- `references/command-router.md`
- `references/command-metadata.json`
- `references/capability-registry.json`
- `references/anti-patterns.md`
- `references/runtime-boundary.md`
- `references/tool-availability.md`

按需读取：

- `references/domain-playbook.md`
- `references/economic-investigation-analysis.md`
- `references/fund-path-and-cash-bridge.md`
- `references/report-schema.md`
- `references/casegraph-roadmap.md`
- `references/plugin-benchmark.md`

禁止默认读取：

- 本地 eval-only golden answer fixture。
- 本地 eval-only rubric/oracle fixture。
- 真实案件数据文件。
- 系统 Codex 配置。

Eval fixture 只能在评测、smoke task 或 oracle 隔离检查中读取。

## 3. 总体执行流

```text
冻结目标与边界
-> 蓝皮书差距审计
-> production path 去样例化
-> 路由与当前案件同步修复
-> Answer Card / Lab Card 输出整改
-> 最小语法与结构测试
-> runtime cache / remount 预检
-> CLI/direct MCP 回归
-> 旧线程行为 replay 与输出污染阻断
-> 真机功能闭环验收
-> Analytix 真机测试
-> 蓝皮书与程序双向校准
-> 发布前核准结论
```

每个阶段都必须输出：

- 已检查内容。
- 发现问题。
- 改动文件。
- 验证命令。
- 验证结果。
- 是否阻断下一阶段。

任何阶段失败，不能跳过。必须先判断失败属于：

- 程序错误。
- 测试断言错误。
- backend/runtime 不可用。
- 蓝皮书缺口。
- 数据缺口。
- 当前案件同步失败。
- 模型 UI 层失败。
- 外部服务或权限问题。

### 3.1 旧线程行为基线原则

旧线程行为基线不是为了复刻旧实现，也不是允许插件用弱来源或未验证计算冒充 source-backed 事实。它用于抽取人工多轮研判已经证明有效的工作行为，并把这些行为产品化为插件测试：

- 能从用户自然语言中识别主体、关联人、关联公司、时间窗和追踪目标。
- 能先确认数据范围、导入清洗覆盖、可用表/字段或 MCP scope，而不是直接写结论。
- 能围绕资金来源、去向、中转、回流、拆分、缺失对手、理财/资产端继续追查。
- 能在用户纠正口径后重新计算，不沿用上一轮错误结论。
- 能对未命中主体、摘要备注、对手户名、联系方式/住址、调单反馈、查控措施、弱匹配和 cleaned/index 差异做否定性复核。
- 能把技术分析结果整理成经侦资金研判材料，而不是把查询步骤、插件说明或内部门禁当成结果。

插件必须保留这些行为中的“侦查自由度”，并把事实获取、证据边界、成本和交付质量收进 MCP、Evidence Ledger、Context Compiler、Claim Verifier 和 Answer Card。若插件因为硬边界、`answer_card_complete=true`、`max_additional_tools=0`、报告门禁或固定事实卡而压制上述行为，视为蓝皮书偏离。

## 4. 阶段一：认知增强差距审计

### 4.1 审计目标

按蓝本逐项找出当前实现偏离点。审计不是为了证明插件已完成，而是为了找出为什么插件没有让 Codex 更会想：哪些事实工具不可见、哪些路由把开放研判压成固定链、哪些输出把内部协议泄漏给普通用户、哪些测试只证明 health 而不证明真实任务功能闭环。

### 4.2 必查能力层

| 蓝皮书层 | 审计问题 | 关键文件 |
| --- | --- | --- |
| 插件身份 | 是否仍是 Skill + MCP + references + scripts + assets，不污染系统 Codex。 | `.codex-plugin/plugin.json`、`.mcp.json`、`SKILL.md` |
| 命令能力矩阵 | 命令、能力族、fallback 是否和 registry 一致。 | `command-router.md`、`command-metadata.json`、`capability-registry.json` |
| 默认 source-backed 能力层 | scope、rank、profile、trace、casegraph、hypothesis、quality、validation 是否默认可发现，低层 raw/debug/write/export/report-heavy 是否受控。 | `tool-discovery-policy.mjs`、`tool-schemas.mjs`、`capability-registry.json`、`tool-availability.md` |
| Controlled Case Workbench | 明确 SQL/notebook/custom口径任务是否能在 Analytix-owned runtime、当前案件、只读、清洗表/分析索引、限行、可回放边界内完成；不可用时是否返回能力缺口而不是伪造计算结果。 | `tool-schemas.mjs`、`tool-call-runtime.mjs`、`command-router.md`、`case-workbench/SKILL.md`、`functional-eval.mjs` 或迁移后的功能测试 runner |
| 自然语言 navigator | `funds_investigate` 是否只是 shortcut / route hint / lightweight navigator，不垄断 Top、画像、穿透、开放深挖。 | `frontdoor-runtime.mjs`、`frontdoor-routing.mjs`、`frontdoor-answer-contract.mjs` |
| 受控意图路由 | 多主体追踪是否进入 Lab/Path，不误入 report gate，且允许 Codex 直接选择对应语义事实工具。 | `intent-plan-protocol.mjs`、`frontdoor-routing.mjs`、`frontdoor-runtime.mjs` |
| Deterministic Runtime | 金额、笔数、账户、户名、对手方是否来自清洗/analysis source、backend、controlled workbench、MCP 承载结果或 Evidence Ledger。 | `safe-skill-runtime.mjs`、`tool-call-runtime.mjs`、diagnostic runtimes |
| Evidence Ledger | 用户可见事实是否有 evidence coverage。 | `evidence-ledger.mjs`、`context-compiler.mjs` |
| Context Compiler | raw payload 是否被压缩成事实卡。 | `context-compiler.mjs`、`card-renderer.mjs` |
| Claim Verifier | 报告 claim、Mermaid、法律敏感词是否能阻断。 | `claim-verifier-protocol.mjs`、`claim-review-diagnostic-runtime.mjs` |
| Investigation Lab | 开放假设是否通用，不依赖固定样例。 | `investigation-lab-*` |
| Fundgraph / Casegraph | supported edge、casegraph context 是否可复核。 | `casegraph-*`、`fundgraph-*` |
| Critique Pass | 反模式是否覆盖普通问答、Lab、图谱、报告。 | `anti-patterns.md`、renderer、diagnostics |
| Eval / Doctor | 是否把 health/golden 误当真实质量证明。 | `doctor.mjs`、`check-health.mjs`、`frontdoor-smoke.mjs`、功能测试脚本 |
| 成熟插件行为 | 是否让 Codex 更会研判，而不是只会调用事实工具。 | `SKILL.md`、`anti-patterns.md`、功能测试脚本、真机功能记录 |
| 能力可发现性 | 多 skill / root 兼容入口下，registry 命令和能力族是否可由 metadata/resource/UI 发现。 | `command-router.md`、`command-metadata.json`、`capability-registry.json`、manifest |
| 工具预算与成本 | 是否通过 registry / Plan DAG 控制重复调用和上下文膨胀，同时不把普通任务压成一次固定前门。 | `capability-registry.json`、`command-metadata.json`、`frontdoor-runtime.mjs`、功能测试脚本 |
| 旧线程行为基线 | 插件是否保留旧线程的直接探索、反复复核、改口径重算、继续追一层能力。 | 本地 session 复盘、`frontdoor-smoke.mjs`、功能测试脚本、真机功能记录 |
| 模型输出污染 | 生产任务是否泄漏蓝皮书、执行方案、doctor、goal、报告门禁模板、diagnostic label。 | `progressive-resources.mjs`、`agent-output-compiler.mjs`、`card-renderer.mjs`、diagnostic runtimes |

### 4.3 审计输出表

每个偏离点必须记录：

| 字段 | 要求 |
| --- | --- |
| `id` | 稳定编号，如 `DRIFT-PROD-SAMPLE-001`。 |
| `blueprint_section` | 对应蓝皮书章节。 |
| `file` | 文件路径。 |
| `line` | 行号或函数名。 |
| `kind` | production runtime / schema example / eval fixture / reference / script / UI bridge。 |
| `risk` | 对真实任务的影响。 |
| `root_cause` | 根因，不写泛泛判断。 |
| `action` | migrate / generalize / delete / keep / document / test-only。 |
| `test` | 修复后如何验证。 |
| `release_blocker` | yes / no。 |

### 4.4 审计命令

固定样例扫描：

```bash
rg -n "<真实主体>|<真实对手>|<真实交易号>|<固定金额>|<固定日期>|<真实机构名>|<真实摘要关键词>" \
  plugins/analytix-fund-analysis/mcp \
  plugins/analytix-fund-analysis/skills/analytix-fund-analysis \
  plugins/analytix-fund-analysis/references
```

eval fixture 隔离扫描：

```bash
rg -n "eval-only fixture|golden answer|golden rubric|oracle" \
  plugins/analytix-fund-analysis/mcp \
  plugins/analytix-fund-analysis/skills/analytix-fund-analysis \
  plugins/analytix-fund-analysis/references
```

工具扩散扫描：

```bash
rg -n "tools/list|specialist|discover|full tool|完整工具|sweep|scan all|全部工具" \
  plugins/analytix-fund-analysis/mcp \
  plugins/analytix-fund-analysis/references
```

当前案件同步扫描：

```bash
rg -n "_analytix|workspaceRoot|case-project.json|case_id|current case|当前案件" \
  packages/runtime/src \
  src/main \
  src/preload \
  src/renderer/src \
  plugins/analytix-fund-analysis/mcp
```

### 4.5 成熟插件借鉴机制核验

蓝皮书的外部借鉴不能停留在“参考过”。审计必须把每个借鉴点落成程序或测试证据：

施工线程必须先实读本机 Data Analytics 的 `index`、`user-context`、`analyze-data-quality`、`validate-data`、`jupyter-notebooks`、`metric-diagnostics`、`visualize-data`、`build-dashboard`、`build-report` 等 focused skills，再写 drift 表。不得用“已参考 Data Analytics”这类空结论代替程序核验。

| 借鉴机制 | 成熟插件原理 | Analytix 经侦增强落点 | 必须产出的证据 |
| --- | --- | --- | --- |
| OpenAI Skills | 短入口 + 渐进披露 + focused workflow；skill 不承担所有上下文。 | `index`、`quick-fact`、`subject-dossier`、`fund-tracing`、`full-case-analysis`、`visual-evidence`、`case-workbench` 等只保留各自 Use When/Workflow/Completion Gate。 | skill 长度、触发条件、reference 一层可达、普通问答不加载全量蓝本的 doctor/静态审计。 |
| Skill Creator（Codex/Anthropic） | `description` 是触发主入口；SKILL body 只放核心工作法；复杂知识进一跳 references；确定性/重复动作进 scripts；用真实任务 forward-test，并对比旧 skill 或无 skill，避免泄漏预期答案和单案过拟合。 | 重构所有 focused skill：frontmatter 覆盖真实经侦触发语，正文短而可执行，references 承载经侦方法、术语、表格/报告样式，scripts 承载 deterministic 检查；成熟插件源码和官方 Skills 机制必须可复核。 | `scripts/skill-creator-alignment-audit.mjs --json --fail-on-gaps` 生成 `skill-creator-alignment.json`：每个 skill 行数、触发描述、必备 gate、引用文件、断链、生产 skill 污染、成熟插件源码可用性和 runtime eval-exclusion 覆盖。 |
| OpenAI Build Plugins / openai/plugins | 插件是可安装、可版本化、可发布的包。 | `.codex-plugin/plugin.json`、`.mcp.json`、skills、references、scripts、assets、Hub package、admin-console 版本同身份。 | manifest/MCP/registry/schema/skill mount/package hash/runtime cache/release identity 证据。 |
| Data Analytics | `index -> user-context preflight -> focused skill -> live source read -> validation -> delivery gate`；SQL/Python/notebook 可用，不靠禁令管住模型。 | `case_source_envelope`、source helper、casegraph/fundgraph、controlled workbench、data-quality、claim-review、visual/report/notebook delivery。 | 15 项等价边界逐项 pass/fail、Pair Amount、Object Dossier、Full Case、Workbench、Visual/Report 真机功能闭环。 |
| Data Analytics `user-context` | preflight 是 source-routing envelope，不是答案来源，也不是总指挥。 | current case project、cleaned/analysis scope、source-of-truth、quality、validation、delivery、gap 统一成 `case_source_envelope`。 | `case-source-envelope.json`，并证明 envelope 不替 Codex 决策、不阻止受控 SQL/Python/notebook。 |
| Data Analytics `validate-data` | 复核问题、方法、来源、计算、图表、结论和未验证项。 | `claim-review` / `analysis-critique` 覆盖普通研判、金额挑战、Lab、图谱、报告，不只查报告末端。 | claim spot-check、金额重算、重复/换卡验证、unsupported flow/法律敏感措辞阻断记录。 |
| Data Analytics `jupyter-notebooks` | notebook 是可执行交付物，不是草稿；必须 top-to-bottom 执行或记录 blocker。 | `case-workbench` / notebook 记录 purpose、scope、SQL/Python、执行状态、artifact、validation_state。 | `workbench-closure.json` 或 notebook artifact，含 executed/blocked 状态和可回放命令。 |
| Data Analytics `visualize-data` / `build-dashboard` / `build-report` | 视觉/报告由交付面 owner 控制，必须有 source、QA 和真实 artifact。 | `visual-evidence` 与 `report-builder` 只从已复核事实生成表格、Top20、图表、看板、附件和公安经侦材料。 | 图表/表格/报告 render 或读取检查；没有 artifact 时输出 blocker，不能用聊天摘要冒充。 |
| Investment Banking | router 只做 admission 和 lead skill selection；lead workflow 拥有 deliverable intake、深度、artifact hierarchy 和 final response；support skills 不替代 banker judgment。 | `index` / root / `funds_investigate` 只做入口和 navigator；`quick-fact`、`subject-dossier`、`fund-tracing`、`full-case-analysis`、`visual-evidence`、`report-builder` 作为 lead workflow owner。 | lead workflow handoff 记录、support-layer hidden 测试、经侦材料 owner 验收；不得由 MCP/card/support text 直接控制最终中文研判。 |
| Public Equity Investing | human-readable hero artifact first；support/audit files 留幕后；PM judgment heuristics 要求事实转为投资判断和下一步动作。 | `investigative_judgment_layer`：金额、主体、流向、全案输出必须有研判意义、异常特征、关系/角色线索、资金用途假设和补证动作。 | `professional_judgment_contract`、`pair_amount_mini_investigation_smoke`、`hero_delivery_contract`，证明不是 fact-card 摘要。 |
| CodeGraph | 预索引图谱给上下文，不替 Agent 判断。 | casegraph/fundgraph 只做主体、账户、对手、交易边、质量门、证据包地图。 | 证明图谱结果带 source/evidence status，candidate edge 不升级为确定路径。 |
| context-mode | raw tool output 留隔离层，模型只看压缩事实。 | raw rows/raw graph/大 JSON/SQL dump 进 artifact、ledger 或 `_meta`，用户正文只保留结论、口径和边界。 | output leak audit；普通研判不出现内部 id、raw rows、大 JSON、debug payload。 |
| Impeccable / Brooks Lint | 领域词汇 + 反模式 + deterministic detector + critique。 | 经侦术语状态、候选/已证/未证、无来源强答、弱来源冒充、bounded preview 总额、单位漂移、法律越界。 | anti-pattern detector、critique pass、普通问答/Lab/图谱/报告覆盖记录。 |
| Superpowers / Aegis | 复杂任务才计划、复核、drift-check；简单任务不被流程压住。 | 全案/报告/重大 QA 用计划和复核；Quick Fact/Pair Amount 用最小充分 source-backed 路径。 | 工具预算和 stop-condition 审计，证明继续追一层/改口径不被完成标志压制。 |
| oh-my-codex | 运行治理、doctor、状态层，不替代 Codex。 | Analytix-owned doctor、runtime cache、case journal、release identity；不改系统 Codex/global config/hooks/cache。 | 隔离审计、runtime cache 同步、Hub package identity、系统 Codex 无污染检查。 |
| AgentOps / Langfuse | trace、eval、反馈回放。 | 只做本地功能闭环、trace、release audit，不接第三方遥测。 | `functional-closure/<timestamp>/` 中 drift、trace、frontdoor、manual UI、release identity。 |
| agentmemory | 有生命周期的状态，不是跨任务事实污染。 | 只保存本案 scope、hypothesis、artifact、journal；不保存跨案事实、固定金额或用户隐私事实。 | case_id 绑定、清理策略、跨案泄漏扫描。 |

#### 4.5.1 14.2 当前逐项核验矩阵（P1 修复后）

本矩阵是本轮 release gate 的事实记录，不是愿景清单。真实案件值只保存在本地闭环输出，不写入 production reference；用户可见交付必须使用公安经侦语言，内部状态只允许留在 `_meta`、audit、doctor、eval、ledger。发布证据必须包含 `P1 env smoke outputs`：真实案件 env-driven P1 smoke 只按 Pass / Partial / Fail 记录任务质量、事实正确、经侦表达和交付物可用性，不用 full_plugin 分数替代。

2026-06-11 复核结论：019eafd0 后续真机问答证明，之前“用户交付层已闭合”的判断过早。当前 source/validation/case-project source/Pair Amount 有改进，但最终回答仍停留在 fact-card 摘要和统计范围解释，缺少 IB/PE 式 lead workflow 交付责任和专业判断层。因此所有涉及用户可见交付、经侦判断、图谱/主体/报告表达的行，在新的真机多轮验收通过前统一降级为 `Partial`，并视为 release blocker。

| 借鉴项 | mature principle | Analytix 落点 | parity/gap | 经侦增强 | 证据文件 | release blocker |
| --- | --- | --- | --- | --- | --- | --- |
| Data Analytics：多 skill / source guardrail / 核验状态 / 交付要求 | `index -> user-context preflight -> focused workflow -> live source read -> validation -> delivery`，SQL/Python/notebook/DuckDB/shell 可作为可复核分析路径，不用工具禁令替代来源治理。 | `skills/index/SKILL.md`、`references/focused-skill-shared.md`、`mcp/frontdoor-runtime.mjs`、`mcp/frontdoor-answer-contract.mjs`、`mcp/agent-output-compiler.mjs`、`mcp/agent-payload-compiler.mjs`。 | Partial：source-backed 路径和输出编译已有基础，但真实问答仍没有形成专业研判交付，不能写成已闭合。 | 公安经侦材料表达：结论先行、事实表格、研判意义、证据边界短写、补证建议；审计层留 `_meta`/ledger。 | `user_delivery_contract`、`user-visible-language-smoke.mjs`、P1/B1/B2 frontdoor smoke 输出、新增 `professional_judgment_contract`。 | Yes：发布前必须通过真机多轮专业交付验收。 |
| Data Analytics：user-context / source envelope | 预检只提供来源路由信封，不替模型作结论；saved context 和 semantic layer 只是候选地图。 | `case_source_envelope` 口径写入 `skills/case-workbench/SKILL.md`、`references/focused-skill-shared.md`、`mcp/agent-context-hygiene.mjs`、`mcp/frontdoor-runtime.mjs`。 | Pass：静态 contract 要求当前案件、事实来源、清洗/analysis scope、质量状态、核验状态、交付状态、gap card；受控 SQL/Python/notebook 未被新增禁令封死。 | 当前案件隔离、摘要缺失单列、重复/换卡风险、资金口径边界、补调材料建议取代商业数据上下文。 | `case_source_envelope_contract`、`case_workbench_control_contract`、`output/analytix-fund-analysis/functional-closure/2026-06-10T06-39-04-972Z/functional-eval.json`。 | No：P1 范围内无阻断；跨案件长期记忆仍不启用。 |
| Data Analytics：validate-data / claim review / critique | 分析验证要覆盖问题、来源、方法、计算、图表、结论和未验证项，不能把报告渲染验证当作事实验证。 | `skills/claim-review/SKILL.md`、`skills/analysis-critique/SKILL.md`、`mcp/user-facing-language.mjs`、`mcp/card-renderer.mjs`、`mcp/agent-output-compiler.mjs`。 | Partial：内部词扫描不等于判断质量通过；当前仍未强制 Top 表、异常扫描、资金意义和下一步动作。 | 用户交付层 critique 覆盖机械口吻、内部标题、无研判意义、表格缺失、图谱不穿透、金额差异解释不可读。 | `user_visible_language_contract`、`output_leak_contract`、`user_delivery_contract`、`professional_judgment_contract`、真机多轮 smoke 记录。 | Yes：critique 必须阻断 fact-card 摘要化输出。 |
| Data Analytics：visualize-data / build-report delivery | 图表和报告必须选择真实交付面、保留来源 metadata、完成最终上下文 QA；不能用聊天摘要冒充附件。 | `skills/visual-evidence/SKILL.md`、`skills/report-builder/SKILL.md`、`references/report-schema.md`、`mcp/fund-flow-graph-runtime.mjs`、`mcp/fundgraph-builder.mjs`、`mcp/card-renderer.mjs`。 | Partial：已有图谱/报告基础，但真实用户反馈仍显示流向图不穿透、主体研判不专业、标题像审计清单。 | 图谱转成资金来源、主要链路、下游去向、资金断点、待补证事项；报告/表格围绕研判结论。 | 资金流向图、单账户研判、图谱表格、全案报告真实 smoke 输出、`hero_delivery_contract`。 | Yes：图谱/主体/报告须通过真实多轮验收。 |
| Investment Banking：lead skill / support hidden / client-ready delivery | router 只选 lead skill；support 只服务 source/model/QC/style；lead workflow 负责第一层真实判断和 hero artifact。 | `index`、root、`funds_investigate` 降为 navigator；focused skills 成为经侦任务 owner；support MCP/card/workbench 只提供证据、验证和 artifact。 | Partial：已有 navigator 方向，但 `funds_investigate` 仍能输出最终中文研判并诱导模型复述，lead owner 边界未闭合。 | 经侦 lead workflow 输出办案材料：两方金额核验材料、主体账户研判、资金穿透图、全案研判报告。 | lead-owner 静态扫描、support-layer hidden 测试、真机最终回答审查。 | Yes。 |
| Public Equity Investing：PM judgment / hero artifact first | 用户先看 human-readable artifact；support/audit files 隐藏；每个实质输出要回答 so what、风险、证据缺口和行动。 | `investigative_judgment_layer`、`professional_judgment_contract`、`pair_amount_mini_investigation_smoke`、`hero_delivery_contract`。 | Partial：当前金额回答只有口径对照，没有足够异常扫描、关系/资金目的研判和下一步侦查路径。 | 把 PM judgment 转为经侦判断：资金性质、角色关系、异常特征、下游去向、补证动作。 | Pair Amount、主体画像、资金流向图、全案报告多轮真机验收。 | Yes。 |
| Data Analytics + IB + PE：专业成稿骨架 | report spine、memo plan、evidence posture、decision hinge、PM judgment 共同保证专业读者可用。 | `案件研判 spine`、`研判材料 plan`、`证据成熟度`、`侦查判断点`、`delivery-qc`。 | New blocker：当前尚未证明每个实质输出都能从正确数据升级为可办案材料。 | 事实表、异常特征、资金意义、角色关系、证据缺口、补证动作按公安经侦语言组织。 | `investigative_spine_contract`、`case_memo_plan_contract`、`evidence_maturity_language_contract`、`delivery_qc_contract`、真机多轮材料审查。 | Yes。 |
| Skill Creator：触发描述 / 渐进披露 / forward-test | 成熟 skill 不是长 prompt，而是“清晰触发 + 短正文 + 一跳 references + 脚本化验证 + 真实任务迭代”。 | 所有 Analytix focused skill 必须按 skill-creator 标准审计；`pair-amount-investigation`、`delivery-qc`、`visual-evidence`、`report-builder` 优先重构。 | Partial：Pair Amount 已开始短 skill 化，但全量 focused skill 尚未完成行数、触发、reference、测试和旧/新输出对照。 | 经侦术语、案件研判方法、报告口吻、图表/附件样式进入 references；正文只保留办案任务的最小工作法。 | `skill_creator_alignment.json`、`pair-amount-contract-smoke.mjs`、`user-visible-language-smoke.mjs`、真实多轮 prompts 输出。 | Yes：长 prompt、硬模板、无 reference 或无真实 forward-test 的 skill 不准视为商业级闭环。 |
| `crystaldba/postgres-mcp`：现场诊断能力 | 吸收 schema/object inspection、restricted safe SQL、只读 wrapper、SQL parser/classifier、count rows、EXPLAIN/diagnostics、top-query/slow-path、database health、query recipes 和审计日志；拒绝任意连接凭据、unrestricted SQL、扩展安装、最终中文结论 owner 和 raw rows 外泄。 | `mcp/duckdb-workbench-runtime.mjs` DuckDB EXPLAIN parser/binder guardrail、`mcp/tool-call-runtime.mjs` evidence-card handoff、`mcp/case-project-context.mjs` 当前案件绑定、`packages/runtime-go/internal/server/runtime_server.go` `_analytix` 注入、`explain_case_sql`、`diagnose_case_sql`、`count_case_rows`、`profile_case_schema`、`preview_case_rows`、`inspect_workbench_history`、`case_sql_recipes`、query-log/slow-path diagnostics、materialized-index/graph health oracle、MCP tool schema/runtime、payload compiler、`database-site-diagnostics.md`。 | Done/Watch：代码级能力已接入受控 Workbench 路线，真实 case-project source、前门多轮专项核算和 MCP return oracle 已覆盖；后续 watch 项是 notebook artifact backend 可用性和慢路径历史样本扩展。 | 数据库现场勘查经侦化：表结构、字段覆盖、记录数核验、清洗范围、查询计划、样本核验、专项核算、可回放证据、图谱端点完整性和索引/派生表健康；样本/计划/profile/count/health 不单独支持报告级金额或链路 claim。 | MCP schema/runtime contract、runtime-go MCP injection tests、tool availability/metadata drift check、case-workbench smoke、functional closure、MCP return oracle graph/materialized-index health。 | Yes：发布前必须通过阻断绕过、无 raw rows 外泄、history 可回放、recipe 可参数化、慢路径可诊断和普通用户语言审查。 |
| `567-labs/instructor`：结构化输出与 validation/reask | 吸收 response_model、Pydantic validation、validation error -> reask、hooks、failed attempts、partial/streaming 和多 provider 抽象；拒绝把它当成模型评分器或事实真值来源。 | `FinalInvestigationAnswer`、`investigation-answer.schema.json`、`mcp/investigation-answer-contract.mjs`、`validate_report_claims(final_investigation_answer)`、`claim-verifier-protocol.mjs`、`delivery-qc`、`report-schema.md`、`investigation-answer-contract-smoke.mjs`。 | Partial：结构化终答合同已落到 schema/validator/smoke/doctor/functional eval，并已进入 report-builder/claim-review 的确定性复核入口；仍需真实多轮验证每类 focused skill 都能把事实包转成该结构再输出自然语言。 | final answer 和报告不显示 JSON，但背后必须有结论、已核验事实、支持资金路径、异常特征、证据边界和补证动作；缺事实必须回 MCP/DuckDB 或降级，不能通过 reask 幻觉补齐。 | `investigation_answer_schema_contract`、`public_security_material_contract`、`investigation-answer-contract-smoke.mjs`、`skill-clause-audit` mature parity、`doctor` claim-review integration。 | Conditional：报告/复核链路已可拦截结构缺陷；发布前仍需真实 Pair Amount、主体画像、资金流向、报告段落按结构合同验收。 |
| 无插件旧线程成功模式：理解 Analytix 软件工作流 | Codex 能在多轮中切换清洗表、analysis 索引、图谱、报告、附件、全文核算和导出，而不是固定问答。 | `analytix_workflow_context`、`case-context`、`case-workbench`、`visual-evidence`、`report-builder`、`delivery-qc`。 | New blocker：当前尚未证明插件能让 Codex 默认理解 Analytix 软件全过程。 | 清洗导出、明细附件、图谱图片、报告续写、金额复核、多轮继续推进都按 Analytix 原生工作流执行。 | `analytix_workflow_context_contract`、`non_template_output_contract`、`cleaned_export_contract`、`report_continuation_contract`、`visual_artifact_contract`。 | Yes。 |
| Impeccable：领域术语、反模式、critique | 专业词汇、反模式 detector、review/critique 应覆盖普通问答、Lab、图谱、报告，而非只看最终报告。 | `skills/quick-fact/SKILL.md`、`skills/case-workbench/SKILL.md`、`skills/fund-tracing/SKILL.md`、`skills/visual-evidence/SKILL.md`、`skills/report-builder/SKILL.md`、`skills/claim-review/SKILL.md`、`skills/analysis-critique/SKILL.md`、`mcp/user-facing-language.mjs`。 | Partial：反模式仍偏词汇清理，未充分阻断“事实正确但研判浅、无 Top 表、无 so what”的低质交付。 | 经侦交付反模式覆盖普通问答、图谱、报告和专题卡，并新增专业判断缺失类反模式。 | `user_delivery_contract`、`analysis-critique` 约束、`professional_judgment_contract`、真机多轮文本记录。 | Yes。 |
| CodeGraph：casegraph / fundgraph | 图谱是上下文和证据索引，不替模型决定侦查结论；候选边不能自动升级为确定路径。 | `mcp/fundgraph-builder.mjs`、`mcp/fund-flow-graph-runtime.mjs`、`mcp/frontdoor-runtime.mjs`、casegraph/fundgraph references。 | Pass：候选边未升级为确定路径，用户图谱表达已转成资金来源、主链、下游去向、断点和待补证事项。 | 资金主链、旁路线索、现金/理财/证券/基金去向、未调取端点以经侦图谱语言和可读图/表呈现。 | 资金穿透图和图谱表格 smoke 输出。 | No。 |
| Superpowers / Aegis：计划、复核、必要分工 | 复杂全案/报告/重大 QA 才使用计划、复核、分工；简单快查必须保持最小充分路径。 | `references/runtime-boundary.md`、`references/top-pluginization-plan.md`、`skills/report-builder/SKILL.md`、`skills/analysis-critique/SKILL.md`、`mcp/frontdoor-runtime.mjs`。 | Pass：策略和边界已落文档与路由，P1 快查未被重型流程拖慢；复杂报告分工只作内部复核，不作为普通用户可见内容。 | 分工只服务分析和复核，用户只见经侦结论、图表、附件和补证建议，不见 workflow/review pass 等内部过程。 | `runtime-boundary.md`、`top-pluginization-plan.md`、P1/full-case/B1/B2 smoke 输出。 | Conditional：P1 不阻断；复杂正式报告需保留内部复核记录。 |
| oh-my-codex：doctor / runtime 状态治理 | 只借运行健康、状态治理和 doctor，不接管系统 Codex，不污染全局配置。 | Analytix-owned doctor、runtime cache、case journal、Hub package、`scripts/prepare-hub-package.mjs`、`scripts/functional-eval.mjs`。 | Pass：本轮修改仅在 `plugins/analytix-fund-analysis`；package isolation 和输出泄漏检查保持本地插件边界。 | 插件运行环境普通用户默认不显示；隔离审计只作为开发/发布材料。 | `hub_package_isolation_contract`、`runtime-boundary.md`、`output_leak_contract`。 | No：未改系统 Codex、全局 `~/.codex`、hooks/MCP/config/cache。 |

#### 4.5.2 复杂任务分工规划与用户可见边界

- 触发范围：复杂全案报告、重大金额争议、资金穿透多层追查、图像/报告 QA 可以拆为事实核算、路径复核、图表/报告质检等内部任务。
- 共享边界：所有分工必须使用同一当前案件、同一数据来源与口径边界、同一核验依据；不得另起事实来源、不得越过只读/清洗表/analysis scope 限制。
- 汇总责任：主 Agent 负责最终研判结论、证据边界、补证建议和经侦材料措辞；分工结果不能直接作为用户可见最终事实。
- 用户可见：只展示基本情况、资金流入流出、重点对手方、异常特征、资金去向、证据边界、补证建议、附件/图表；不展示内部任务分工、review pass、workflow、工具过程或调试标签。

核验结论必须写成：

- 已有程序落点。
- 缺失程序落点。
- 与 Data Analytics 或对应成熟插件的 parity / gap。
- 经侦增强点是否超过借鉴对象的通用能力。
- 对应测试或 doctor gate。
- 是否影响本轮 release blocker。

不允许的核验写法：

- 只写“已对齐 Data Analytics”，没有引用本机 skill 的机制和 Analytix 程序落点。
- 只跑脚本分数或旧 A/B，对真实 Pair Amount、Object Dossier、Full Case、Workbench、Visual/Report 没有功能产物。
- 把 `rank_counterparties`、casegraph、scope map、bounded preview 或语义说明当最终 source-of-truth。
- 把缺失来源、未执行 SQL、未渲染 artifact、未同步 current case project 写成通过。

### 4.6 能力可发现性审计

插件页只显示 `Analytix Fund Analysis` 或单个 root skill，在 MVP 阶段不自动构成失败；但商业成熟验收必须补齐 Data Analytics 式 focused skill 产品面。失败在于用户和模型无法通过 skill、命令、metadata 和 resources 发现对象画像、全案分析、资金穿透、报告复核、证据表格/图表/看板/附件等能力矩阵。

能力可发现性必须审计：

- `.codex-plugin/plugin.json` 的 displayName、description、defaultPrompt 是否表达商业资金研判能力，而不是内部测试或说明书。
- `SKILL.md` 是否明确语义事实工具箱、`funds_investigate` navigator、资金研判边界、主动研判原则和渐进披露资料。
- `command-router.md` 是否保留 `/analytix` 稳定命名空间、registry 命令、fallback、普通问答/报告/专题线索/Case Workbench 分流规则。
- `command-metadata.json` 是否把命令、lane、门禁、预算、输出契约机器化。
- `capability-registry.json` 是否是能力、工具、eval、budget 的上游事实源。
- `progressive-resources.mjs` 与 doctor required resources 是否暴露必要生产契约。
- 插件页 UI 能否让开发者理解资金穿透、主体画像、开放深挖、证据复核、报告交付能力，而不是只看到一个泛化技能名。

可发现性通过标准：

- 不为了显示“很多 skill”而拆出浅层 skill。
- 有明确 focused skill 路线图或已落地 skill：`index`、`case-context`、`quick-fact`、`account-dossier`、`subject-dossier`、`fund-tracing`、`investigation-lab`、`full-case-analysis`、`report-builder`、`evidence-request`、`analysis-critique`、`claim-review`、`graph-visualization`、`visual-evidence` 等。
- 不让用户通过背命令才能使用插件，自然语言资金研判任务必须可触发。
- registry 中声明的命令和能力族能由 command metadata/resource/插件描述发现并校验。
- 能力发现资料不包含 eval fixture、真实案件事实、oracle、raw rows 或本机 runtime 路径。

### 4.7 旧线程行为基线审计

审计线程必须读取并结构化复盘旧线程行为，但不得把真实案件事实、敏感主体、交易明细、账号、凭证、token 或 raw rows 写入本文档、蓝皮书或生产 reference。旧线程只提供行为基线：

| 行为基线 | 审计问题 | 失败信号 | 对应插件层 |
| --- | --- | --- | --- |
| 导入清洗覆盖确认 | 插件能否确认 source/norm/clean/index 覆盖、异常行、去重口径和 left tree 聚合。 | 只输出“已导入”但无覆盖口径、无异常行边界、无复核动作。 | Deterministic Runtime、Casegraph、Data Quality Card |
| 多主体资金追踪 | 能否围绕主体甲/乙及关联人/公司形成主体池、账户池、共同对手、中转、回流和资产端线索。 | 输出工具说明或报告门禁；只列 Top，不追通道；无法给下一步追查。 | Intent AST、Plan DAG、Investigation Lab、Fundgraph |
| 用户改口径重算 | 用户要求扩大/收窄范围、补充摘要备注、包含缺失对手或剔除重复时，能否重新计算。 | 复用旧答案、只解释口径、不重算、不标记前后差异。 | Continuation State、Evidence Ledger、Context Compiler |
| 否定性搜索 | 对“是否出现在对手户名/摘要备注/联系方式/住址/调单反馈/查控措施/cleaned/index/弱匹配”能否给出已查范围和未命中边界。 | 只说“未发现”但不说明查了哪些字段和清洗数据层。 | QA Review、Search Probe、Claim Verifier |
| 继续追一层 | 对“这笔钱前后再追一层”能否沿 supported edge 或候选断点给出前后手、时间差、金额差、补调对象。 | `answer_card_complete=true` 后停止；把候选路径画成确定链。 | Fundgraph、Path Probe、Lab Card |
| 经侦材料化 | 能否把技术分析整理成事实、线索、证据缺口、补证清单和报告段落。 | 输出 SQL 思路、插件教程、内部 id、diagnostic label。 | Answer Card、Report Schema、Critique Pass |

审计输出必须形成 `old_thread_behavior_drift` 表，至少包含：

- `behavior_id`
- `source_thread_id`
- `expected_plugin_behavior`
- `current_plugin_behavior`
- `missing_runtime_capability`
- `missing_test`
- `release_blocker`

如果无法读取历史 session 文件，不能跳过该基线；必须从已有报告产物、用户复述和蓝皮书目标中抽取同等行为基线，并把“session 不可读”记录为证据缺口。

## 5. 阶段二：Production Path 去样例化

### 5.1 原则

生产 runtime 不允许包含固定案件事实。插件可以有领域规则，但不能有固定案件默认事实。

允许存在：

- schema 中的通用示例，但建议改成 `示例主体`、`示例对手方`。
- eval fixture 中的固定 golden。
- smoke task 参数中的固定 task。
- 本地功能测试输入文件中的测试任务。

禁止存在：

- production runtime 默认主体。
- production runtime 默认对手方。
- production runtime 默认交易号。
- production runtime 默认金额。
- production runtime 默认报告 claim。
- production runtime 默认时间窗。
- Context Compiler 默认注入固定事实。
- Answer Card 默认注入固定案例文字。

### 5.2 分类处置规则

| 发现类型 | 处置 |
| --- | --- |
| `holder_name || "固定人名"` | 删除默认人名，改为无 holder 时使用 `focus_keywords` 或 `当前案件主体`。 |
| `keywords: ["固定人名", ...]` | 改为领域关键词；固定人名由 task 参数传入。 |
| 固定交易号 | 只能来自 backend 返回事实；无返回则写“未稳定返回，不得补写”。 |
| 固定金额 | 只能来自 backend 返回事实；无返回则写 facts 缺口。 |
| 固定时间窗 | 只能来自用户显式参数或任务上下文；不能默认设置案件日期。 |
| 固定 claim review | 迁入 eval fixture 或 smoke task；生产 claim review 读取用户提供的 report text / claims。 |
| tool schema 示例 | 可保留但建议改为“示例主体/示例对手方”，避免误导。 |
| card renderer 固定输出 | 通用化为“重复放大风险/unsupported flow/候选归属风险”。 |

### 5.3 必查生产模块

优先清理：

- `mcp/investigation-lab-diagnostic-runtime.mjs`
- `mcp/claim-review-diagnostic-runtime.mjs`
- `mcp/destination-diagnostic-runtime.mjs`
- `mcp/frontdoor-routing.mjs`
- `mcp/intent-plan-protocol.mjs`
- `mcp/frontdoor-runtime.mjs`
- `mcp/agent-output-compiler.mjs`
- `mcp/card-renderer.mjs`
- `mcp/frontdoor-answer-contract.mjs`
- `mcp/tool-schemas.mjs`
- `mcp/tool-input-schemas.mjs`

### 5.4 迁移目标

固定 golden 内容迁移到：

- 本地 eval-only golden answer fixture。
- 本地 eval-only rubric/oracle fixture。
- `scripts/frontdoor-smoke.mjs` 的 task 参数。
- 本地 eval-only prompt suite；若仍使用旧命名脚本，必须先确认它只作为功能任务 runner，不执行旧打分路线。

迁移后 production runtime 只能依赖：

- 当前 `case_id`。
- current case-project binding。
- 用户问题。
- 显式 `holder_name` / `via_holder_name` / `account` / `focus_keywords`。
- 显式 `date_start` / `date_end`。
- Capability Registry。
- MCP/backend 返回的事实。

### 5.5 去样例化完成标准

生产目录扫描结果必须分级：

- P0：production runtime 中仍有固定样例驱动逻辑，阻断。
- P1：用户可见文案可能泄漏固定样例，阻断或必须改。
- P2：tool schema 示例，有误导风险，建议改为通用示例。
- P3：eval fixture / smoke task，允许保留。

P0/P1 必须清零。P2 可以列入后续，但商业发布前建议清理。

## 6. 阶段三：路由与当前案件同步

### 6.1 路由目标

真实用户任务：

```text
围绕 A 及 A 关联人/关联公司、B 及 B 关联人/关联公司，
全面资金梳理，找到资金关联通道，进行资金追踪。
```

必须识别为：

| 维度 | 正确结果 |
| --- | --- |
| Task Profile | `multi_subject_trace` / `open_trace_investigation` |
| Capability | `investigation-lab` + `subject-outflow-tracing` + `pattern-and-asset-leads` |
| Intent | Lab / trace / path，而非 report gate |
| Output | Investigation Card + Flow/Continuation next actions |

错误结果：

- `full_case` 报告门禁。
- `write_blocked` 模板。
- 插件说明书。
- 只给下一步不出事实。
- 输出无关主体或固定 golden。

### 6.2 路由修复点

检查：

- `intent-plan-protocol.mjs`
- `frontdoor-routing.mjs`
- `frontdoor-runtime.mjs`
- `command-metadata.json`
- `capability-registry.json`

修复规则：

- “全面”不等于报告。
- “资金梳理/追踪/穿透/关联通道/回流/中转/拆分/共同对手/关联人/关联公司”优先进入 Lab / Path。
- 只有用户明确说“生成报告/撰写报告/正式材料/报告初稿/附件/报告复核”才进入 report lane。
- 显式 `/analytix report` 进入 report lane。
- 无明确报告意图但出现多主体追踪时，不得触发 claim review 作为主输出。

### 6.3 当前案件同步目标

链路必须闭合：

```text
analytix 案件项目工作区
-> .analytix/case-project.json
-> analytix runtime _analytix.workspaceRealPath context
-> MCP case resolution
-> funds_investigate case_id
```

用户已经在案件中心选中案件时，Agent 不应常态化要求用户再次提供 `case_id`。

### 6.4 当前案件失败处理

| 状态 | 输出 |
| --- | --- |
| `.analytix/case-project.json` 与 DuckDB 可读 | 直接使用当前案件，输出 scope；后端 API 不可达只作为 warning。 |
| runtime `_analytix.workspaceRealPath` 或完整冻结授权字段缺失 | 在工具执行前拒绝，并由宿主返回固定 Case Source Blocker。 |
| 显式 `case_id` 与案件项目绑定冲突 | 显式写 scope warning 并阻断跨案取数。 |
| 无任何当前案件 | 阻断事实输出，提示需要选择案件；不得猜默认案件。 |
| DuckDB 缺失或不可读 | 输出 runtime failure，不写 0 值或无数据。 |

### 6.5 最小验证

当前案件 API：

```bash
curl -sS http://127.0.0.1:18731/.analytix/case-project.json + case DuckDB binding
```

激活案件 API：

```bash
curl -sS -X POST http://127.0.0.1:18731/api/v1/cases/<case_id>/activate
```

前端构建：

```bash
cd /Users/sun/Projects/analytix
npm run build
```

## 7. 阶段四：Answer Card 与 Investigation Lab 输出整改

### 7.1 输出目标

插件输出必须像研判材料，不像工具说明。

必须包含：

- 当前案件与 scope。
- 主体范围。
- 账户范围。
- 主要资金收付。
- 共同交易对手。
- 资金关联通道。
- 疑似中转 / 回流 / 拆分。
- 缺失对手 / 现金断点。
- 理财 / 资产端线索。
- 已证实事实。
- 高可信线索。
- 待复核假设。
- 禁止写成事实。
- 证据缺口。
- 下一步追查队列。

### 7.2 禁止输出形态

- 只写“我可以帮你分析”。
- 只列工具能力。
- 只写报告门禁。
- 只写 `write_blocked=true`。
- 只给下一步建议，没有事实结果。
- 出现无关固定 golden 主体。
- 输出内部 id、artifact id、audit ref。
- 将 unsupported flow 画成确定性 Mermaid。
- 把候选账户写成确认归属。
- 把缺失对手写成真实对手方。

### 7.3 Answer Card 最小字段

每张卡至少具备：

| 字段 | 要求 |
| --- | --- |
| `case_id` | 当前案件。 |
| `intent` | 路由后的意图。 |
| `scope` | 主体、账号、时间窗、方向、数据范围。 |
| `key_facts` | 确定性事实。 |
| `hypotheses` | 假设及状态。 |
| `evidence_status` | supported / partial / missing / needs_review。 |
| `warnings` | 边界与风险。 |
| `forbidden_as_facts` | 禁止事实化内容。 |
| `next_actions` | 可执行追查动作。 |
| `answer_card_complete` | 是否足以回答本轮问题。 |

### 7.4 Lab Card 状态机

```text
用户侦查目标
-> 假设队列
-> 确定性事实 probe
-> 证据状态分级
-> 可疑线索集合 / 金额集中 / 时间集中 / 相关账户（列明账号）
-> unsupported claim 降级
-> 下一步追查
-> 收口边界
```

任何假设必须是：

- 已证实。
- 高可信线索。
- 待复核。
- 已排除。
- 禁止写成事实。

### 7.5 Output Rule Registry 与 Critique Pass

Answer Card / Lab Card 不是单纯的展示层。它们必须把 `anti-patterns.md` 和 command metadata 中的输出规则产品化，避免插件只在报告阶段才复核风险。

必须进入 critique pass 的输出类型：

| 输出类型 | 必查反模式 |
| --- | --- |
| 普通金额问答 | bounded rows total、单位漂移、账户/户名错配、重复交易号放大。 |
| 排名/主体画像 | 候选归属升级、成功交易口径缺失、方向口径缺失、时间窗缺失。 |
| 资金链路/图谱 | unsupported flow、无证据边、聚合排行冒充交易链、Mermaid 确定化假设。 |
| Investigation Lab | 假设不分级、缺口被写成事实、下一步不可执行、固定样例泄漏。 |
| 报告/附件 | unsupported claim、法律敏感词过度定性、金额/笔数/账户无 evidence。 |

整改要求：

- `frontdoor-answer-contract.mjs` 或输出编译层必须表达每类 answer card 的必填字段和 forbidden fields。
- `card-renderer.mjs` 不得把内部 id、artifact ref、audit ref、debug label 作为用户证据。
- `claim-verifier-protocol.mjs` 继续承担报告硬门禁，但普通研判也必须有 deterministic anti-pattern gate。
- 若 critique pass 命中阻断项，输出应给出事实缺口、降级表述和下一步补证动作，而不是只写 `write_blocked=true`。

### 7.6 Answer Card 自主研判与收口边界

`answer_card_complete=true` 只能表示“本轮问题已有足够事实形成 bounded answer”，不能表示“开放式追查应永久停止”。不同任务必须使用不同收口边界：

| 任务类型 | `answer_card_complete` 规则 | `max_additional_tools` 规则 | 失败信号 |
| --- | --- | --- | --- |
| 普通金额/账户问答 | 返回精确口径、金额/笔数/账户、边界后可为 true。 | 通常 0。 | 缺口明显却阻止复核。 |
| Top 排名 | 覆盖排序口径、方向、时间窗、去重边界后可为 true。 | 通常 0-1。 | 只给一个 Top 口径，缺少排序口径。 |
| Object Dossier | 某卡/某人研判必须给出账户/主体范围、时间跨度、进出账、Top 对手、异常特征、来源/去向、证据缺口和下一步后才可为 true。 | 允许计划化追加 targeted rank/profile/trace/hypothesis/quality。 | 对象研判只输出一个金额、Top 或普通摘要。 |
| 多主体追踪/资金穿透 | 只有给出主体池、账户池、关键通道、证据状态、下一步追查队列后才可为 true。 | 不得默认 0；若仍有高优先级假设，应允许计划化追加。 | “全面追踪”任务只输出一张保守事实摘要。 |
| Investigation Lab | 假设队列、状态、证据缺口、stop conditions 均完整后才可为 true。 | 由 Plan DAG 决定；不能用固定 0 压制继续追一层。 | 用户要求继续深挖时重复上一卡或拒绝追查。 |
| 报告/报告复核 | 通过 source/data quality/mandatory review/claim review 后才可出正式报告级文本。 | 最多 `funds_investigate + validate_report_claims`，但普通研判不得套用报告门禁。 | 普通追踪题输出 `write_blocked` 模板。 |

整改要求：

- `card-renderer.mjs` 和 `agent-output-compiler.mjs` 必须事实优先、边界次之、指令最后；不得让“最终回答必须”“报告门禁”压过事实内容。
- 报告门禁短语只能在明确报告级任务、报告 claim 复核或正式报告文本中出现。
- 开放式深挖卡必须给出可执行下一步：补调对象、字段、时间窗、金额集中、账户/对手、交易号或证据缺口。
- 若事实不足，应返回 `partial`、`needs_review` 或 `continue_recommended`，不得用完整卡掩盖事实缺口。
- 用户多轮追问时，Continuation State 必须保留前一轮 scope、已验证事实、被排除假设和新增统计范围差异。

## 8. 阶段五：最小测试

本阶段停止把历史线程或长链路分数当作事实依据。`019ed600` 只能作为历史
现象参考，不作为当前质量结论；`full_plugin`、完整八轮和全案报告只在发布
候选阶段使用，不能成为每次小改的默认验证。

### 8.0 商业级测试金字塔

| 层级 | 验收目标 | 最小证据 | 发布含义 |
| --- | --- | --- | --- |
| L0 静态闭环 | 蓝本、执行方案、skills、metadata、registry、MCP manifest、runtime cache 合同一致。 | `doctor`、reference `cmp`、`node --check`、`git diff --check`。 | L0 不过不得进入任何用户前门验收。 |
| L1 确定性事实测试 | MCP/DuckDB 取数正确，current case project 同步正确，pair amount、rank、profile、trace、graph 与受控 SQL 互相核验。 | 当前案件 deterministic smoke、pair amount contract、只读 SQL 对账记录。 | L1 不过不得回答金额、排名、画像、链路类结论。 |
| L2 路径自主测试 | MCP 足够时走最小 MCP；MCP 不足时 Codex 能自主选择最小充分 source-backed 路径，可用 source helper/MCP、casegraph/fundgraph、受控 SQL/Python/notebook/workbench、图表/报告/附件和后置 critique。 | 短链路任务记录工具选择、事实来源、补充查询原因和停止条件。 | L2 不过说明插件仍在用工具摘要替代模型判断。 |
| L3 真实前门短链路 | 单问、三轮、多轮追问 canary 真实解决问题。 | 前门记录覆盖简单事实、追问画图、主体研判、全案概览、报告片段。 | L3 不过不得进入 RC。 |
| L4 发布候选长链路 | 完整八轮、全案报告、发布包、安装/卸载/回滚只在 RC 候选运行。 | RC evidence 目录、包 hash、doctor、release audit、必要长链路。 | L4 不是小改烟测，也不能替代 L1-L3。 |
| L5 专家人工复核 | 经侦专家和一线民警视角判断专业、可读、深入，无机械词、无错误建议、无内部实现外泄。 | 人工复核记录、用户可见话术审查、剩余 P0/P1。 | L5 不过不得发布给普通办案用户。 |

测试顺序必须从 L0 到 L5。任何层级只能证明本层事实，不得用评分替代真实案件
核验；发现 source-of-truth、事实粒度、重复/换卡、同事实去重、用户可见话术
问题时，应回到对应最小层修正。

### 8.1 语法测试

对修改过的 `.mjs` 文件运行：

```bash
node --check plugins/analytix-fund-analysis/mcp/<file>.mjs
node --check plugins/analytix-fund-analysis/scripts/<file>.mjs
```

### 8.2 diff 检查

```bash
git diff --check
git status --short
```

### 8.3 reference 同步

```bash
cmp -s \
  plugins/analytix-fund-analysis/references/top-pluginization-plan.md \
  plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/top-pluginization-plan.md

cmp -s \
  plugins/analytix-fund-analysis/references/blueprint-execution-plan.md \
  plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-execution-plan.md

cmp -s \
  plugins/analytix-fund-analysis/references/blueprint-implementation-matrix.md \
  plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-implementation-matrix.md
```

### 8.4 doctor

```bash
node plugins/analytix-fund-analysis/scripts/doctor.mjs --json
```

如果 doctor 因新增 reference 报错：

- 检查 `runtime-cache-contract.mjs`。
- 检查 root/skill reference 是否一致。
- 检查 progressive resources。
- 检查 public copy wording。

### 8.5 后端 API 可用性

CLI/direct MCP 测试前必须确认 backend：

```bash
curl -sS http://127.0.0.1:18731/api/v1/health
```

如果 18731 不可用，不能把 smoke 失败判成插件逻辑失败。可临时启动 backend：

```bash
cd /Users/sun/Projects/analytix
../.venv/bin/python -m uvicorn app.main:app --host 127.0.0.1 --port 18731
```

测试完成后停止该进程。

### 8.6 runtime cache / remount 预检

真实 UI 失败常见原因是 workspace 文件已经修复，但 `analytixagent` 实际加载的 Analytix-owned runtime cache 仍是旧插件。进入真机功能验收前，必须明确 runtime cache 状态。

只读预检：

```bash
node plugins/analytix-fund-analysis/scripts/sync-runtime-cache.mjs --json
```

本地 remount 同步只允许在用户确认这是本地开发/RC remount 时执行：

```bash
node plugins/analytix-fund-analysis/scripts/sync-runtime-cache.mjs \
  --apply \
  --confirm-local-remount \
  --json
```

边界：

- `sync-runtime-cache.mjs` 只同步 Analytix-owned runtime cache；它不是 Hub 安装或官网发布工具，也不改系统 Codex。
- doctor 的 runtime cache drift warning 在真机前必须解释清楚；若真机要验证最新插件，应先消除相关 drift。
- 不能把 workspace 测试通过当成 UI 加载最新插件的证明。
- runtime cache 同步后必须重新跑 `doctor.mjs --json`，并记录 runtime home 与同步文件清单。

### 8.7 eval coverage 与工具预算预检

最小测试还必须证明测试覆盖没有遗漏能力主线：

```bash
node plugins/analytix-fund-analysis/scripts/eval-coverage-contract.mjs
node plugins/analytix-fund-analysis/scripts/functional-eval.mjs --list-tasks --json
```

通过要求：

- capability registry、command metadata、eval task 覆盖关系无漂移。
- 评测任务覆盖普通问答、排行、Object Dossier、Full Case Analysis、资金穿透、Lab、报告复核、passive 非介入。
- 每类任务有工具预算，不允许用重复调用和大上下文换分数。
- 功能任务 runner 的默认 holder/date 示例只能作为 eval prompt 默认值；真实任务必须显式传入主体、时间窗或从当前案件上下文获取。若仓库暂未存在 `functional-eval.mjs`，施工线程必须先从旧 runner 迁移或增加兼容入口，不能继续沿用旧打分语义。

### 8.8 真实线程 rollout 审计

真实 UI 人工复测只是入口，必须继续解析 Analytix-owned runtime 下的 rollout JSONL。审计必须量化每轮工具调用、MCP payload 字节数、模型可见 payload 字节数、JSON/转义 JSON 次数、重复 JSON、内部字段泄漏、shell 本地猜案、最终回答经侦材料质量和失败归因。

```bash
node plugins/analytix-fund-analysis/scripts/agent-thread-locator.mjs --recent 10 --json
node plugins/analytix-fund-analysis/scripts/agent-thread-audit.mjs --recent 5 --json
```

若要审计指定线程：

```bash
node plugins/analytix-fund-analysis/scripts/agent-thread-audit.mjs \
  --id analytix-agent://threads/<thread-id> \
  --json
```

通过要求：

- 搜索范围默认只包含 Analytix-owned runtime；只有显式只读调查时才使用 `--include-system-codex`。
- 不能只看最终回答；`json_like_model_visible_output_count`、`large_model_visible_output_count`、`internal_field_leak_output_count` 必须作为发布判断。
- shell 读取本地案件目录、扫描 `*.duckdb`、从历史输出猜 `case_id` 必须归因到 current-case/source guardrail，而不是归因到“模型发挥”。
- 真实线程使用的 runtime 插件版本必须与候选源码/Hub package 身份匹配；不匹配时只作为旧版本失败样本。

## 9. 阶段六：CLI / Direct MCP 回归

### 9.1 作用边界

CLI/direct MCP 测试只证明：

- MCP server 可启动。
- backend 可达。
- 路由和事实卡可生成。
- 样例泄漏可被 forbidden markers 捕获。
- Answer Card 基础结构有效。

它不能证明：

- analytix 案件项目 UI 插件安装成功。
- 研判助手真实模型会正确调用插件。
- 当前案件 UI 选择已同步。
- GPT5.5 超高推理下最终答复质量达标。

### 9.2 fixture 隔离与旧线程行为模式回归

```bash
node plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs \
  --backend-url http://127.0.0.1:18731 \
  --tasks old_thread_patterns \
  --json \
  --timeout-ms 180000
```

通过要求：

- `pass_count=1`
- `fail_count=0`
- direct card 无 auto-fail、无说明书化输出，并能回答真实任务。
- 不出现 opaque refs。
- 该项只验证旧线程行为模式、fixture 隔离和固定样例不泄漏；不得作为发布级质量分数或真实功能闭环替代品。

### 9.3 真实语义工具 / navigator 环境变量任务

真实案件信息不得写入脚本。用环境变量注入：

```bash
ANALYTIX_FUNDS_REAL_FRONTDOOR_CASE_ID='<case_id>' \
ANALYTIX_FUNDS_REAL_FRONTDOOR_QUESTION='<用户真实资金追踪任务>' \
ANALYTIX_FUNDS_REAL_FRONTDOOR_FOCUS_KEYWORDS='<主体A,主体B>' \
ANALYTIX_FUNDS_REAL_FRONTDOOR_MARKERS='<必须出现的用户问题主体或意图短语>' \
ANALYTIX_FUNDS_REAL_FRONTDOOR_FORBIDDEN_MARKERS='<固定golden主体,固定交易号,无关样例金额>' \
node plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs \
  --backend-url http://127.0.0.1:18731 \
  --tasks real_multisubject_trace_lab \
  --json \
  --timeout-ms 180000
```

通过要求：

- `pass_count=1`
- 用户主体标记命中。
- forbidden markers 不出现。
- 输出不是 report gate 模板。
- 缺失事实写成 facts 缺口，不补写固定样例。

### 9.4 check-health

```bash
node plugins/analytix-fund-analysis/scripts/check-health.mjs \
  --backend-url http://127.0.0.1:18731 \
  --case-id <case_id> \
  --deep \
  --json
```

解释规则：

- `ok=true` 只证明 runtime 健康，不证明真实任务质量。
- eval fixture skipped 不能作为发布级质量证明。
- 若 health 通过但真实语义工具 / navigator 任务失败，以真实任务为准。

### 9.5 CLI/direct MCP 通过后的阻断条件

即使 fixture 隔离回归、真实语义工具 / navigator 任务和 check-health 都通过，以下情况仍然阻断进入发布核准：

- `doctor.mjs --json` 仍提示 runtime cache 中生产文件漂移，且真机测试准备使用 runtime cache。
- real semantic/navigator 输出只给说明或下一步，不给事实、线索、缺口和追查队列。
- real semantic/navigator 仍要求用户提供 `case_id`，但 current case-project binding 有值。
- 输出命中 forbidden markers、内部 task id、oracle、rubric、diagnostic label。
- 工具调用多次重复同一事实查询，且没有新的假设或证据增量。
- raw payload、raw rows、大 JSON 或 artifact detail 进入模型正文。

这些阻断条件必须回到对应蓝本层处理，不能通过放宽断言、缩短用户任务或删除 forbidden marker 解决。

### 9.6 旧线程行为 Replay Suite

CLI/direct MCP 通过后，必须执行旧线程行为 replay。Replay 不是把旧线程案件事实写入脚本，而是用环境变量或 fixture metadata 注入当前测试案件、主体、时间窗和 forbidden markers，复刻旧线程的工作行为。

必测任务：

| task_id | 测试目的 | 输入要求 | 通过要求 |
| --- | --- | --- | --- |
| `replay_import_clean_coverage` | 验证导入/清洗/分析覆盖复核能力。 | case_id；可选 source category。 | 返回 source/norm/clean/index 覆盖、异常行、去重口径、left tree 或主体聚合边界；不输出 raw rows。 |
| `replay_multisubject_trace` | 验证多主体资金梳理和通道追踪。 | case_id；主体甲/乙；关联关键词。 | 输出主体范围、账户范围、主要收付、共同对手、中转/回流/拆分、资产端线索、缺口和下一步追查。 |
| `replay_rescope_recompute` | 验证用户改口径后重算。 | 初始问题；追加口径变更。 | 明确前后统计范围差异，重新计算或标记缺失，不沿用旧答案。 |
| `replay_negative_search` | 验证否定性复核。 | 主体/关键词；字段范围。 | 说明已查数据层和字段；未命中必须给出边界和下一步补调。 |
| `replay_continue_one_more_hop` | 验证继续追一层。 | 一条 supported edge 或候选断点。 | 给出前手/后手、时间差、金额差、证据状态、补调对象；候选路径不得确定化。 |
| `replay_report_materialize` | 验证经侦材料化输出。 | 已有事实卡或 trace card。 | 输出事实、线索、证据缺口、补证清单和材料段落；不得输出 SQL/插件教程/内部 id。 |
| `replay_amount_challenge` | 验证金额被质疑后的复核。 | 一项金额/笔数 claim；用户质疑。 | 重新按口径复核，说明可支持金额、不可支持金额和纠正口径。 |
| `replay_full_case_simple_prompt_runs_tree` | 验证短句“全案分析/跑完整功能树”不会收缩成计划。 | case_id；可选业务镜头。 | 默认执行 full-case fact tree，覆盖数据量、时间跨度、清洗覆盖、Top20、可疑特征和续调清单。 |
| `replay_source_trace_upstream` | 验证资金来源/上游追溯。 | seed account/subject/transaction 或可解析对象。 | 输出 visible upstream、supported/candidate 边界、断点和补调对象，不写最终来源。 |
| `replay_economic_crime_typology_leads` | 验证经侦 typology 只做侦查镜头。 | case_id；脱敏案类或专题提示。 | 输出 feature family、supporting facts、evidence status、downgrade reason、next proof，不输出法律定性。 |
| `replay_case_delivery_pack` | 验证办案交付包。 | 已有事实卡或 full-case 输出。 | 输出事实、主体/账户/对手方、特征、流向、补调、claim QA 的材料结构，不输出工具日志。 |

执行方式：

```bash
ANALYTIX_FUNDS_REPLAY_CASE_ID='<case_id>' \
ANALYTIX_FUNDS_REPLAY_SUBJECTS='<主体A,主体B>' \
ANALYTIX_FUNDS_REPLAY_QUESTION='<真实任务的脱敏问题>' \
ANALYTIX_FUNDS_REPLAY_FORBIDDEN_MARKERS='<固定golden主体,固定交易号,固定金额,报告门禁模板>' \
node plugins/analytix-fund-analysis/scripts/functional-eval.mjs \
  --tasks replay_import_clean_coverage,replay_multisubject_trace,replay_rescope_recompute,replay_negative_search,replay_continue_one_more_hop,replay_report_materialize,replay_amount_challenge,replay_full_case_simple_prompt_runs_tree,replay_source_trace_upstream,replay_economic_crime_typology_leads,replay_case_delivery_pack \
  --json
```

若 `functional-eval.mjs` 尚不存在或尚不支持这些 task，施工线程必须先从旧 runner 迁移/包裹出功能回放入口，扩展 eval task registry、capability registry 映射和断言，不能把缺失测试当作通过。

Replay auto-fail：

- 输出插件说明书、蓝皮书、执行方案、`/goal`、doctor、内部评测标签、diagnostic label。
- 输出 `write_blocked` 或报告门禁模板，但用户任务不是正式报告复核。
- 出现固定 golden 主体、固定交易号、固定金额或固定时间窗。
- 只给下一步建议，没有任何当前事实、线索或边界。
- 用户改口径后未重新计算，也未说明无法计算的 deterministic 原因。
- 把候选路径、缺失对手、控制/代持/最终归属写成事实。

Replay 失败后的修复映射：

| 失败类型 | 回到插件层 |
| --- | --- |
| 无法识别主体/关联范围 | Intent AST、Command Router、Task Profile。 |
| 无法给事实 | Deterministic Runtime、Casegraph、Fundgraph。 |
| 无法继续追一层 | Plan DAG、Fundgraph、Trace Runtime、Lab Card。 |
| 改口径不重算 | Continuation State、Evidence Ledger、Context Compiler。 |
| 输出说明/门禁 | Answer Card、Card Renderer、Agent Output Compiler。 |
| 样例泄漏 | Production Path 去样例化、Eval isolation。 |

### 9.7 模型输出污染阻断 Suite

真实失败的一个关键形态是模型拿到过多内部说明、门禁短语或施工文档后，把用户任务回答成插件说明。必须单独测试模型可见输出污染。

阻断测试必须覆盖：

| 检查项 | 通过要求 |
| --- | --- |
| progressive resources | 普通案件研判不得默认读取 `top-pluginization-plan.md`、`blueprint-execution-plan.md`、eval rubric 或 oracle。 |
| agent-readable tool text | 首段必须是事实/结果/边界，不得以“最终回答必须”“报告门禁”“协议字段”开头。 |
| report gate scope | `write_blocked`、`validate_report_claims`、报告门禁短语只出现在报告级任务。 |
| internal id leakage | 不输出 `q_xxx`、`audit_ref`、`artifact_id`、diagnostic label、task id。 |
| blueprint/doc leakage | 不输出“蓝皮书”“执行方案”“/goal”“doctor”“内部评测标签”“测试分数”等施工语汇。 |
| golden leakage | 不输出与当前案件无关的固定主体、交易号、固定金额、固定日期。 |

建议命令：

```bash
node plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs \
  --backend-url http://127.0.0.1:18731 \
  --tasks real_multisubject_trace_lab \
  --forbid-doc-leakage \
  --forbid-report-gate-leakage \
  --json \
  --timeout-ms 180000
```

若现有脚本没有 `--forbid-doc-leakage` 或 `--forbid-report-gate-leakage`，必须补齐等价断言。不能只靠人工肉眼判断。

## 10. 阶段七：Analytix 真机测试

### 10.1 为什么必须真机测试

真实失败发生在：

```text
Analytix 案件中心
-> 选择案件
-> 研判助手
-> 插件页显示已安装
-> 新建对话
-> GPT5.5 超高推理
-> 用户真实资金追踪任务
```

CLI 不能覆盖该链路。因此最终必须启用 Analytix 做真机测试。

### 10.2 启动

```bash
cd /Users/sun/Projects/analytix
ANALYTIX_FORCE_WEB_BUILD=1 bash scripts/dev/run.sh
```

### 10.3 测试前检查

在 UI 中确认：

- 案件中心能看到目标案件。
- 当前案件已选中。
- 研判助手页可打开。
- 插件页显示 `Analytix 涉案资金研判` 已安装。
- 插件 skill / capability 描述能让用户理解资金穿透、主体画像、开放深挖、报告复核能力。
- 当前 runtime/backend health 正常。

### 10.4 真机测试任务

输入同一类任务：

```text
围绕【主体A】及【主体A】关联人、关联公司，
以及【主体B】及【主体B】关联人、关联公司，
进行全面资金梳理，找到资金关联通道，进行资金追踪。
要求输出主体范围、账户范围、主要收付、共同交易对手、疑似中转/回流/拆分、理财/资产端线索、缺失对手、证据缺口和下一步追查队列。
```

### 10.5 真机通过标准

必须满足：

- 不要求用户重复提供 `case_id`，除非当前案件确实没有选中或同步失败。
- 能说明当前案件 scope。
- 使用插件 MCP 能力；普通语义任务不得把弱来源或未验证计算写成 source-backed 事实，明确自定义口径可走受控 Case Workbench。
- 输出真实研判结果。
- 不输出说明书。
- 不输出固定 golden 样例。
- 不把假设写成事实。
- 有下一步追查队列。
- 对事实缺口能明确降级。

### 10.6 真机失败分类

| 失败现象 | 归因 | 处理 |
| --- | --- | --- |
| 要求用户提供 `case_id` | case-project source 同步失败 | 修 UI/backend/runtime 同步链路。 |
| 输出说明书 | skill / answer compiler / model instruction 失败 | 修 root skill、Context Compiler、Answer Card。 |
| 输出 report gate | 路由失败 | 修 Intent AST / Command Router。 |
| 出现固定样例 | production runtime 污染 | 去样例化。 |
| 无法调用插件 | 安装/manifest/MCP 问题 | 修 plugin package / runtime cache / Hub package。 |
| 事实为空但写无风险 | runtime 失败处理错误 | 修 tool error / partial / missing 边界。 |
| 输出过长但无结果 | Context Compiler 失败 | 修卡片压缩和必填字段。 |
| 真实功能闭环失败 | 架构未落地 | 回到 blueprint drift 审计。 |

### 10.7 真机功能闭环预验收

真机功能闭环不是对比实验，也不是追求脚本分数。它只回答一个问题：当前 Analytix 内嵌 Agent + 当前插件包 + 当前 runtime cache + 当前案件数据，能否按 Data Analytics 式 `source + validation + delivery` 治理完成真实经侦资金任务。

预验收必须覆盖：

| 功能场景 | 必须证明 |
| --- | --- |
| Quick Fact / Pair Amount | 能识别 controlling source、主体/账户/对手粒度、去重/换卡/同事实风险；排行工具不得单独控制最终金额。 |
| Object Dossier | 某卡/某人资金研判能输出基本情况、开户/身份/联系方式/住址线索、账户结构、进出账、Top 对手、异常特征、来源/去向、缺口和下一步。 |
| Full Case Analysis | “全案分析/跑完整功能树”默认执行全案分析树，不收缩成计划或报告门禁。 |
| Trace / Lab | 继续追一层、改口径重算、否定性搜索和开放深挖不被 stop flag 压制。 |
| Visual / Report / Workbench | 图表、报告、notebook、附件包必须有真实 artifact、render/execute QA 或明确 blocker。 |
| Passive | 非案件资金任务不误触发案件工具。 |

每个场景必须记录当前版本、HEAD、runtime home、runtime cache 状态、current case project 来源、runtime/backend health、用户输入、用户可见输出、关键 source envelope、validation state、delivery state 和失败分类。不能用旧产物、平均分、旧 golden、工具调用日志或 direct MCP JSON 替代真实前门结果。

### 10.8 当前案件同步链路验收

真机失败中“请提供 case_id”属于高优先级阻断。进入人工真机功能验收前，必须对当前案件同步链路形成可回放证据：

```text
Analytix 案件中心选中案件 / 启动时默认恢复的上一次案件
-> web caseProjectCaseId / last-opened case state
-> current case-project binding / current case endpoint
-> analytix runtime _analytix.workspaceRealPath context
-> analytix_funds MCP case resolution
-> semantic tool result / navigator answer case_id/scope
```

验收项：

- UI 选中或进入案件项目后，`.analytix/case-project.json`、runtime `_analytix.workspaceRealPath`、`caseBindingHash` 和案件 DuckDB 快照返回一致 `case_id`。
- analytix 冷启动恢复案件项目时，必须自动带上 current case-project binding；不得要求用户返回案件中心重新选择已经打开的案件。
- 进入 Agent、新建对话、刷新页面、切换案件、runtime cache remount 后，都要有同一套 case-project source 解析证据。
- 新建研判助手对话时，analytixagent 能拿到 Analytix-owned runtime context。
- 语义事实工具和 `funds_investigate` navigator 在未显式传入 `case_id` 时能解析当前案件。
- 若同步失败，输出必须写明失败链路位置，而不是常态化要求用户回去再选案件。
- 同步失败不得被模型改写成“用户未提供 case_id”。
- 同步失败时不得列目录、搜索 `*.duckdb`、读取 Downloads/case、本地扫描 `/Users`，也不得从历史输出、fixture 或测试残留中猜测 `case_id`。
- 显式 `case_id` 不存在时必须返回 invalid case blocker，不得连续尝试其他候选 id。
- 真机功能闭环产物必须保存 case-project source 检查命令、接口返回摘要、插件最终输出和失败分类。

## 11. 阶段八：功能闭环、产物与发布身份

### 11.1 功能闭环目的

本阶段不再跑旧式对比实验，也不再以旧打分表作为发布依据。目标是证明当前插件版本在真实 Analytix 运行链路下，能按 Data Analytics 母版完成 `source + validation + delivery` 闭环：来源可控、计算可复核、状态可解释、交付物真实存在或有明确 blocker。

必须覆盖的闭环场景：

| 场景 | 通过要求 |
| --- | --- |
| Pair Amount | “A 转给 B 多少钱”按 controlling source、主体/账户/对手粒度、同事实/换卡/重复候选核算，不能由排行结果单独最终作答；必须证明占位交易号、空交易号、非唯一交易号不会单独控制去重；必须分列 raw、effective/dedup、本金/手续费可选口径、duplicate/unsupported amount，并给出账户/时间集中、差异解释和下一步追踪。 |
| Object Dossier | 某卡/某人资金研判完成基本情况、账户结构、进出账、Top 对手、异常特征、来源/去向、证据缺口和下一步。 |
| Full Case Analysis | 全案短句能执行全案分析树，覆盖数据量、时间跨度、清洗覆盖、进出账、按人/账户统计、Top20、可疑特征和续调清单。 |
| Trace / Lab | 继续追一层、改口径重算、否定性搜索、开放深挖保留侦查自由度，并标注 supported/candidate/blocked。 |
| Workbench | 语义工具不足时可用当前案件、只读、清洗/analysis scope、限行、purpose、validation、artifact 边界执行 SQL/Python/notebook。 |
| Visual / Report | 表格、图表、notebook、附件包、报告必须有真实 artifact、render/execute QA 或 blocker，聊天摘要不能冒充交付。 |
| Passive | 非案件资金任务不调用案件工具。 |

### 11.2 功能闭环产物目录

所有真机、CLI/direct MCP、doctor、runtime cache、workbench 和发布身份结论必须落到同一当前版本产物目录，不能只在对话里口头说明。

建议目录：

```text
/Users/sun/Projects/analytix/output/analytix-fund-analysis/functional-closure/<timestamp>/
```

至少包含：

| 文件 | 内容 |
| --- | --- |
| `execution-summary.md` | 本轮目标、修改范围、阻断项、通过项、剩余风险。 |
| `blueprint-drift-table.json` | 阶段一 drift 表，含旧路线残留删除记录。 |
| `mcp-decentralization-audit.json` | MCP 是否替 Codex 决策、是否已从 production 面删除或降级。 |
| `case-source-envelope.json` | 当前案件 source/preflight envelope 样例和字段覆盖。 |
| `pair-amount-closure.json` | Pair Amount / Amount Challenge 核算、去重/换卡/同事实验证状态；必须包含占位交易号去重键安全检查、竞争口径 reconciliation、禁止接受错误 effective amount 的断言、最终用户可见答复质量检查。 |
| `workbench-closure.json` | workbench 成功或 gap 的目的、scope、只读边界、验证状态和 artifact。 |
| `skill-clause-audit.json` | 对照 Data Analytics、Investment Banking、Public Equity Investing 等成熟插件，逐条审核 skill 的 router、source guardrail、focused owner、completion gate、经侦语言和过度限制风险。 |
| `mcp-return-oracle.json` | 不跑大模型评分，直接用真实当前案件 DuckDB 只读 oracle 核验 MCP/Workbench 的 schema、count、graph/evidence base tables、aggregate、profile、preview、diagnose、EXPLAIN 和安全阻断返回是否正确。 |
| `mcp-surface-snapshot.json` | 不跑大模型评分，直接请求 MCP initialize、tools/list、resources/list/read、prompts/list/get，确认 Agent 插件脚手架、渐进披露资源和数据库/图表/报告 workflow prompts 闭环。 |
| `doctor.json` | `doctor.mjs --json` 输出。 |
| `runtime-cache-sync.json` | runtime cache 预检或 remount 后输出。 |
| `frontdoor-functional.json` | 真实前门功能任务输出。 |
| `replay-functional.json` | 旧线程行为功能 replay 输出。 |
| `agent-thread-audit.json` | 真实 Analytixagent rollout JSONL 审计：tool calls、payload bytes、JSON/重复 JSON、内部字段泄漏、最终回答质量和失败归因。 |
| `output-leak-audit.json` | 模型输出污染阻断结果。 |
| `case-sync-trace.json` | 当前案件同步链路验收记录。 |
| `manual-ui-notes.md` | 人工真机步骤、截图位置、观察结果、失败分类。 |
| `release-identity.json` | manifest、MCP server、registry、tool schema、skill mount、Hub package hash、runtime cache、HEAD/tag/admin-console 版本一致性。 |
| `changed-files.txt` | 修改文件清单。 |
| `validation-commands.txt` | 已运行命令和退出结果。 |

产物不得包含真实案件敏感原始数据、raw rows/raw graph/raw query dumps 全量内容、token/cookie/provider config、系统 Codex 全局配置、eval oracle 或 golden answer。

### 11.3 成本与工具预算验收

成本不是只看总耗时，还包括工具调用次数、上下文长度、重复查询、raw payload 暴露和人工返工。Data Analytics 式路线要求普通任务轻量、风险任务升级、交付任务强验证。

预算原则：

| 任务类型 | 预算口径 |
| --- | --- |
| 普通金额/账户问答 | Codex 选择一个最小充分语义事实工具；模糊问题可先用 `funds_investigate` navigator；不得多工具扫库。 |
| Top 排名 | 直接使用 rank 语义工具；重复调用必须有新 metric、direction、time window 或 scope。 |
| Object Dossier | 某卡/某人研判允许组合 profile、rank、trace、hypothesis、quality，但每步必须填充完整画像包中的具体字段。 |
| 多主体追踪/Lab | 允许计划化多步，但每步必须对应假设、事实 probe、证据缺口或下一步动作。 |
| 正式报告/报告复核 | 复杂报告进入 report lane，汇总 scope/quality/rank/trace/lab facts 后再 `validate_report_claims`；不能替代普通研判。 |
| 非资金任务 | passive，不调用案件工具。 |

成本失败信号：

- 同一轮对同一主体/同一口径重复调用相同工具。
- 用完整低层工具发现替代 Capability Registry 路由。
- 用单一 `funds_investigate` 固定链替代 Codex 对语义事实工具的自主选择。
- 把 raw rows 或大 JSON 交给模型自己归纳。
- 为了凑结论进行大范围无计划扫描。
- 没有 Answer Card 压缩，导致模型上下文被细碎事实淹没。

成本验收必须进入功能闭环产物、doctor/功能测试或人工真机记录，不得只靠主观描述。

### 11.4 Rust 触发边界

执行方案不要求把插件大规模改成 Rust。Rust 只在性能、确定性或跨平台 native 边界有证据时进入。

应优先考虑 Rust 的场景：

- 大规模交易聚合、去重、金额集中/时间集中计算。
- fundgraph/casegraph 构建、投影、布局、node/edge 合并。
- 高内存导入/清洗辅助、批量 transforms。
- 跨平台 packaged native compute 或 runtime bridge。
- 需要 deterministic lint / detector 且 JS/Python 性能不足的规则。

不应使用 Rust 的场景：

- 改写稳定 Python 业务逻辑只为统一语言。
- 报告文本、提示词、用户交互、UI 展示。
- 未经 profiling 的泛化重构。
- 会扩大 macOS/Windows 打包风险的临时实现。

Rust 进入前必须有：

- 现有瓶颈证据。
- 输入/输出契约。
- 最小可替换边界。
- 跨平台打包计划。
- 对应 doctor/eval 或 fixture。

## 12. 阶段九：蓝皮书同步修正

### 12.1 什么时候改蓝皮书

必须改蓝皮书的情况：

- 真机测试暴露蓝皮书没有覆盖的运行链路。
- 蓝皮书约束过度，压制了 Codex 的侦查思考。
- 蓝皮书某项定义导致真实功能闭环失败或压制 Codex 自主研判。
- 新增能力、命令、runtime、eval、doctor、resource 需要纳入系统边界。
- 测试暴露 release-quality functional closure 定义不足。

不应该改蓝皮书的情况：

- 只是为了让测试通过。
- 只是为了保留固定样例。
- 只是为了允许生产 runtime 读取 golden。
- 只是为了放松 source、validation 或 delivery gate。
- 只是为了让报告更快生成而牺牲事实复核。

### 12.2 蓝皮书修正要求

每次修正必须说明：

- 原蓝皮书章节。
- 发现的问题。
- 程序或测试证据。
- 修正内容。
- 是否影响 reference / runtime / doctor / eval。
- 是否同步 skill 内嵌 reference。

### 12.3 同步命令

```bash
cp plugins/analytix-fund-analysis/references/top-pluginization-plan.md \
   plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/top-pluginization-plan.md

cp plugins/analytix-fund-analysis/references/blueprint-execution-plan.md \
   plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-execution-plan.md
```

## 13. 阶段十：发布前核准

发布前必须具备：

- P0/P1 production 样例污染清零。
- doctor 通过或只剩明确非阻断 warning。
- fixture 隔离与旧线程行为模式回归通过，且不作为真实前门功能闭环替代品。
- real semantic/navigator 环境变量任务通过。
- check-health deep 通过，并清楚说明 eval fixture skipped 不作为真实质量证明。
- runtime cache drift 已消除，或明确本轮不进入 UI/真机验证且已说明原因。
- eval coverage / model task list 已覆盖能力主线和工具预算。
- 发布前真实前门功能闭环已通过，或因 app-server/鉴权环境阻断并有说明；该产物必须绑定当前插件版本、当前 HEAD、runtime cache preflight、app-server、backend、client cwd、current case project、真实前门 turn 和用户可见输出。
- Analytix 真机测试通过。
- 能力可发现性通过：多 skill 产品面、command metadata、resources、插件页能力描述能支撑 registry 命令和能力族。
- critique pass 覆盖普通问答、排行、图谱、Lab 和报告，不只覆盖正式报告。
- 功能闭环产物完整，能回放 doctor、runtime cache、smoke、health、人工真机、source envelope、validation state、delivery state。
- root references 与 skill references 一致。
- runtime cache contract 包含新增 reference。
- package source 不含 eval fixture。
- 无系统 Codex 污染。
- 无未解释 dirty worktree。

如果任一条件不满足，不能发布。

## 14. 常见问题处理

### 14.1 后端 API 不可用

现象：

```text
fetch failed
curl 127.0.0.1:18731 failed
```

处理：

- 不把 smoke 失败判成插件逻辑失败。
- 先确认端口。
- 临时启动 backend 或启动完整 Analytix。
- 重新跑 smoke。

### 14.2 fixture 隔离或旧线程行为模式回归失败

处理顺序：

1. 看是否 后端 API 不可用。
2. 看是否 marker 过期。
3. 看事实是否仍正确。
4. 如果是测试措辞漂移，修 marker。
5. 如果事实缺失，修 runtime。
6. 不允许把固定样例写回 production runtime。
7. 不允许把该类 fixture 回归包装成真实功能闭环或发布质量分数；真实质量仍以当前案件、当前 runtime、真实前门和 source + validation + delivery 产物为准。

### 14.3 真实语义工具 / navigator 任务失败

处理顺序：

1. 检查当前 case_id 是否有效。
2. 检查路由是否进 Lab/Path。
3. 检查 forbidden markers 是否命中。
4. 检查输出是否只有说明/门禁。
5. 检查 fact missing 是否被正确降级。
6. 失败必须进入 drift 表，不能只调 prompt。

### 14.4 Analytix 真机要求 case_id

处理：

- 检查 UI `caseProjectCaseId`。
- 检查 backend `/.analytix/case-project.json + case DuckDB binding`。
- 检查前端是否调用 `bindCaseProject`。
- 检查 agent runtime 是否读取 current case-project binding。
- 修同步链路，不把“请提供 case_id”作为正常流程。

### 14.5 插件输出说明书

处理：

- 检查 root skill 是否过度说明。
- 检查 card renderer 是否把门禁放在结果前。
- 检查 Answer Card 是否缺少 `key_facts`。
- 检查 Context Compiler 是否只返回 warnings。
- 检查 model final 是否忽略事实卡。
- 增加真机功能验收 auto-fail：输出主要是工具说明则失败。

### 14.6 真实功能闭环失败

处理：

- 不先调 prompt。
- 对照 source、validation、delivery、workflow、cost 维度定位是哪一层弱。
- 若深挖弱，修 Investigation Lab / fundgraph。
- 若事实弱，修 deterministic runtime。
- 若工具选择弱，修默认 source-backed 能力层、tool discovery policy、tool descriptions 和 capability registry，不把问题推给模型提示词。
- 若回答机械，先检查 `funds_investigate` 是否仍是总指挥或固定链路，再检查 `agent-output-compiler.mjs`、`destination-diagnostic-runtime.mjs`、`fund-flow-graph-runtime.mjs`、`graph-visualization/SKILL.md`、`subject-dossier/SKILL.md` 是否把审计合同、固定小标题或“必须原样保留”指令放进普通用户可见路径。
- 若输出弱，修 Context Compiler / Answer Card。
- 若成本高，修 Capability Registry / Plan DAG。
- 若自由探索被压制，修自主研判保留条款、casegraph 感知、hypothesis probe 和后置 critique。

### 14.7 runtime cache 与 workspace 不一致

现象：

```text
doctor warning: local runtime install cache drift
```

处理：

- 不把 workspace 文件检查通过当成 UI 已加载最新插件。
- 先运行 `sync-runtime-cache.mjs --json` 观察漂移。
- 若要进入真机功能验收，经用户确认后执行 `--apply --confirm-local-remount`。
- apply 后重新跑 doctor，并把输出放入功能闭环产物。
- 不能用该脚本替代 Hub 发布、安装或升级链路。

### 14.8 插件页只有一个 skill

处理：

- 不把“只有一个 root skill”在 MVP 阶段直接判定为失败；但商业成熟验收必须补 focused skill 产品面。
- 检查 root/index skill 是否短入口。
- 检查是否已经规划或落地 Data Analytics 式 focused skills：`case-context`、`quick-fact`、`account-dossier`、`subject-dossier`、`fund-tracing`、`investigation-lab`、`full-case-analysis`、`report-builder`、`evidence-request`、`analysis-critique`、`claim-review`、`graph-visualization`、`visual-evidence`、`case-workbench` 等。
- 检查 `command-router.md` 的 registry 命令和 fallback 是否完整。
- 检查 `command-metadata.json`、`capability-registry.json`、progressive resources 是否可发现。
- 检查插件页 displayName/description/defaultPrompt 是否表达资金研判能力。
- 若能力不可发现，修 manifest、focused SKILL、metadata、resources；不要为了视觉数量拆出虚假 skill。

### 14.9 真机功能环境失败

处理：

- 如果 app-server 无法启动、WebSocket 不可用或模型鉴权失败，标记为环境阻断。
- 保留真机功能 runner 输出和日志。
- 不把环境阻断计入插件质量分，但不能据此发布。
- 继续使用 CLI/direct MCP、manual UI notes 和功能闭环产物定位程序问题。

### 14.10 Rust 方案争议

处理：

- 先看是否存在性能、确定性、跨平台 native 或高内存证据。
- 若只是 prompt、报告写作、UI 展示或稳定业务逻辑，不进入 Rust。
- 若进入 Rust，必须先写清输入/输出契约、替换边界、打包影响和测试。
- Rust 不能成为绕过 Evidence Ledger、Context Compiler 或 Claim Verifier 的新事实层。

### 14.11 旧线程 replay 失败

处理：

- 先判断失败是测试脚本缺能力、source helper / workbench / MCP 缺事实、路由错误、输出污染还是模型 UI 层失败。
- 若旧线程行为代表真实人工多轮研判已经证明有效，而插件无法完成，不能降低 replay 断言；必须回到蓝皮书的自主研判保留条款、Investigation Lab、Fundgraph、Continuation State 修复。
- 若 replay 涉及真实敏感数据，不把原始数据写入测试 fixture；使用环境变量、脱敏 marker、验证摘要和功能闭环产物。
- 若 replay 输出只给建议、不出事实，优先修 Answer Card 和 Context Compiler，而不是追加更多工具说明。
- 若 replay 因缺少脚本任务失败，先补 `functional-eval.mjs` / `frontdoor-smoke.mjs` task 和断言。

### 14.12 模型输出污染

处理：

- 若普通研判输出蓝皮书、执行方案、doctor、goal、内部评测标签、报告门禁模板，先检查 `progressive-resources.mjs`、root skill 渐进披露、`agent-output-compiler.mjs` 和 `card-renderer.mjs`。
- 生产任务的 agent-readable text 必须事实优先；内部协议、字段要求和门禁短语只能作为 debug 或明确报告级任务附属信息。
- 不能靠在 prompt 里追加“不要输出说明书”解决；必须减少模型默认可见的内部说明。
- fixed golden 文案必须迁移到 eval-only fixture，生产路径只能表达通用风险类别。

### 14.13 收口边界压制追查

处理：

- 若用户要求“继续追一层”“全面梳理”“开放深挖”，但工具返回 `answer_card_complete=true/max_additional_tools=0` 后模型停止，检查这些字段是否被用户可见化或被当成跨轮禁令。
- 对多主体追踪、Lab、资金穿透类任务，`answer_card_complete` 只能表示本轮 bounded answer 完成；仍有高优先级假设时，必须给下一步追查队列、可选语义工具和 stop condition。
- 修复时优先调整 `funds_investigate` navigator、tool choice contract、Lab Card、Trace Runtime、Plan DAG、Case Workbench 和 answer contract，不允许让模型把弱来源或未验证计算写成生产事实。

### 14.14 用户交付层机械化或审计合同外露

现象：

- 最终答复事实基本正确，但开头是工具预检句。
- 答复出现同名收款人内部聚合标题、核心账号内部标题、来源/数据审计标题、图谱依据表标题、`direct/candidate` 原词或候选数量机械标签。
- 图谱答案像验证清单，不像资金穿透图；主体画像像工具输出，不像经侦研判材料；金额题只解释口径，不分析大额交易、异常特征、资金意义和下一步核查。

处理：

1. 不先调 prompt，不把问题归因于模型口吻；先查 production agent-readable text 是否把内部审计合同交给模型。
2. 检查 `destination-diagnostic-runtime.mjs` 的 fact label 和 answerText；内部 label 可保留到 `_meta`/ledger/debug，不得成为用户标题。
3. 检查 `agent-output-compiler.mjs` 是否输出“必须保留”“不要合并或改名”“最终回答必须...”等指令型文本；这些只能进入测试/doctor，不得进入普通用户答复。
4. 检查 `graph-visualization/SKILL.md` / `fund-flow-graph-runtime.mjs` 是否强制审计小标题；改成读者路径：资金来源、主要资金链路、下游去向、资金断点、待补证事项。
5. 检查 `subject-dossier/SKILL.md` 是否仍要求 `Subject Dossier`、`direct / candidate`；改成经侦口吻：账户基本情况、登记账户、待核账户线索、重点账户、重点对手方、异常特征、核查建议。
6. 增加负向测试，禁止普通最终回答出现本节列出的机械标题和英文/internal label。
7. 增加正向测试，要求金额题、资金流向图、主体账户研判和报告片段都 answer-first、表格化、经侦化、有研判意义和下一步核查。
8. 若脚本通过但真机文本仍像审计清单，判为 release blocker；不得用 smoke 分数、direct MCP 正确或旧 A/B 产物替代真实用户交付质量。

### 14.15 专业判断层缺失或 fact-card 摘要化

现象：

- 最终答复只复述金额、笔数和两个口径，缺少 Top 大额交易表、异常特征、关系/角色研判、资金目的线索和下一步侦查动作。
- 答复看似事实正确，但读者仍不知道“这笔钱在案件里意味着什么”“为什么异常”“下一步查哪里”。
- MCP/card/workbench 的 support text 直接塑造最终答案，lead focused skill 没有承担交付责任。
- 图谱、主体画像、全案材料有标题和数字，但缺少可直接用于办案讨论的结论、表格、分析和补证建议。

处理：

1. 对照本机 `Investment Banking`：检查 `index` / root / `funds_investigate` 是否越权做实质业务；router/navigator 必须只选择 lead workflow，不拥有最终答案。
2. 对照本机 `Public Equity Investing`：检查最终回答是否有 hero deliverable 和 judgment layer；support/audit files 不得成为用户正文。
3. 在 `quick-fact`、`subject-dossier`、`fund-tracing`、`full-case-analysis`、`visual-evidence`、`report-builder` 中补齐 Use When / Workflow / Output Contract / Completion Gate，明确它们是用户交付 owner。
4. 在 `analysis-critique` 增加专业判断层质检：无 Top 表、无异常扫描、无资金意义、无下一步核查、只说“统计范围不同不能合并”均 fail。
5. 在 `agent-output-compiler.mjs` / `frontdoor-answer-contract.mjs` / `card-renderer.mjs` 中把 support facts 与 final delivery 分层；工具可提供结构化证据，但不得输出最终中文研判模板来诱导模型复述。
6. Pair Amount 最小合格输出必须包括结论句、期间、金额/笔数、集中日/集中账户、大额交易表、异常特征、竞争金额业务解释、下游追踪建议。缺任一核心块时，普通真机验收判 fail。
7. 测试不得只检查“不出现内部词”；必须检查“是否像公安经侦研判材料、是否可用于继续追查、是否有读者价值”。

## 15. Historical `/goal` 执行提示词（inactive）

> 本节仅保留历史 prompt 来源，不是当前可执行指令。不得从本节推导
> Goal、写入、Git、发布或验收授权。

本章负责把“重施工合同”留在 blueprint 内，把真正粘贴到新线程的
`/goal` 压缩到 4000 字内。执行线程不得把短提示词理解成降低验收标准；
短提示词只是入口，所有 drift、测试、隔离、功能闭环和发布身份要求仍以本文件
第 0.5、4、11、14、15 章以及 `top-pluginization-plan.md` 为准。

### 15.1 新线程读取合同

新线程必须优先读取：

- `plugins/analytix-fund-analysis/references/top-pluginization-plan.md`
- `plugins/analytix-fund-analysis/references/blueprint-execution-plan.md`
- `plugins/analytix-fund-analysis/references/capability-registry.json`
- `plugins/analytix-fund-analysis/references/command-router.md`
- `plugins/analytix-fund-analysis/references/command-metadata.json`
- `plugins/analytix-fund-analysis/references/tool-availability.md`
- `plugins/analytix-fund-analysis/references/anti-patterns.md`
- `plugins/analytix-fund-analysis/.codex-plugin/plugin.json`
- `plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md`
- `plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-execution-plan.md`
- `plugins/analytix-fund-analysis/mcp/tools/tool-discovery-policy.mjs`
- `plugins/analytix-fund-analysis/mcp/tools/tool-schemas.mjs`
- `plugins/analytix-fund-analysis/scripts/doctor.mjs`
- `plugins/analytix-fund-analysis/scripts/functional-eval.mjs` 或待迁移的旧功能测试 runner

### 15.2 P0 drift 必查清单

施工线程必须先确认并修复这些旧路线残留：

- `SKILL.md` 仍把 `funds_investigate` 写成 single default front door。
- 插件仍只有一个大而全 skill，且蓝本、manifest、doctor、eval 没有 Data Analytics 式 focused skill 产品面。
- “某卡/某人资金研判”仍被归入普通问数或排行，而不是 Object Dossier。
- “全案分析/完整研判/按功能树跑完”仍被归入报告门禁或普通摘要，而不是 Full Case Analysis。
- Full Case Analysis 的机器契约仍等同 `/analytix plan`，或自然语言全案分析只给 lane plan 不执行 `run_full_case_analysis(write_report=false)`。
- 经侦 typology、异常特征库、来源追溯、negative search、金额质疑包或办案交付包没有进入 focused skills、registry/metadata、eval 和输出质检。
- `capability-registry.json` 仍只默认暴露 3 个工具或保留
  `max_default_visible_tools=3`。
- `tool-discovery-policy.mjs` 仍用 slice/数量上限截断默认工具箱。
- `tool-availability.md`、doctor 或 eval 仍把“默认三工具”当作正确状态。
- 裸 `/analytix` 仍无条件进入 full_case/report gate。
- `funds_investigate` 仍替 Codex 垄断 rank/profile/trace/hypothesis/quality。
- “A 转给 B 多少钱”“金额范围对吗”“两个金额范围哪个对”等 Pair Amount / 金额质疑仍由 `rank_counterparties` 排行金额直接作答，没有核验 source-of-truth、holder/account scope、counterparty grain、raw/effective/dedup 口径、时间窗、同事实/换卡风险和 targeted validation / workbench。
- Pair Amount / Workbench 使用占位、空值、`查无信息`、`unknown`、`无`、`null` 或明显非唯一交易号作为唯一去重键，导致不同交易被错误合并或重复换卡未识别。
- Pair Amount 最终答复只给一个摘要金额，没有说明 raw/effective/本金/手续费/unsupported amount 的统计范围差异、相关账户（列明账号）、时间集中、重复组、下一跳或质量边界。
- `answer_card_complete` / `max_additional_tools` 被模型当作跨轮禁令，
  压制“继续追一层”“改口径重算”“开放深挖”。
- 普通研判输出蓝本、执行方案、doctor、内部评测标签、diagnostic label、
  report gate、write_blocked、内部 id、raw rows、大 JSON 或 golden/oracle。
- casegraph/fundgraph 把 unsupported candidate flow 画成确定性证据。
- 当前案件同步只能靠用户重复提供 `case_id`，不能从 current case project resolution
  自动获得。
- Analytix 启动时已默认打开上一次案件，但 current case-project binding 未同步，Agent 首跳工具报 `current case project is not set` 后开始搜索本地文件、试探历史/测试 case_id。

### 15.3 默认 source-backed 能力层合同

默认可见工具箱不是“越多越好”，而是必须让 Codex 自由选择高价值语义事实工具。
默认可见至少覆盖：

- scope/current case：`get_current_case`、`get_case_scope_map`、`get_scope_coverage`
- graph：`get_casegraph`、`build_fund_flow_graph`
- rank：`rank_accounts`、`rank_holders`、`rank_counterparties`
- profile：`analyze_account_full`、`analyze_holder_full`
- trace：`trace_subject_top_outflows`、`trace_fund_next_hop`、`trace_fund`
- reasoning support：`hypothesis_probe`、`audit_case_data_quality`
- validation：`resolve_duplicate_families`、`validate_continuation_list`、
  `validate_report_claims`
- workbench：`run_case_sql`、`create_case_notebook` 或等价受控执行能力，仅在语义工具不足、自定义口径、金额挑战或可回放计算需求下条件可见
- navigator：`funds_investigate`

raw rows、debug payload、写入、导出、完整低层发现、全案报告重链路只允许在
开发诊断、显式授权、专项 eval 或正式 report lane 下可见。

### 15.4 旧 `/goal` 清理说明

旧 15.4 / 15.5 `/goal` 正文已从执行方案中删除。删除原因：旧稿包含
上一阶段授权边界，以及容易把测试引回旧对比实验、旧打分和旧修补路线的可粘贴
文本。当前文件只保留一个正式
施工入口：15.6「全权授权发布版 `/goal` 施工合同」。如需追溯旧稿，只看 git
历史，不能把旧稿作为当前施工依据。

### 15.6 全权授权发布版 `/goal` 施工合同（接近 3900 字）

本节是当前最新正式施工入口，取代 15.5。详细施工仍以前文蓝本、执行方案、
15 项 Data Analytics 等价软边界、14.2 借鉴机制落地矩阵、doctor/功能测试/
release guard 和真实运行功能闭环产物为准。新线程可以获得用户全权授权，自主决定
commit、push、tag、生成发布包和通过 `https://analytix.top/admin-console`
发布最新版，但发布只能发生在所有发布级 blocker 清零之后；不能伪造发布、
不能用本地 remount 冒充官网发布、不能绕过认证或人工登录。

#### 15.6.1 新线程详细施工依据

1. 基准路线必须完全对齐 Data Analytics 的 `source + validation + delivery`
   治理：只阻断无来源强答、弱来源冒充、未验证计算、假交付、内部实现外泄。
   Data Analytics 没有封锁的分析工具，Analytix 不自创工具封锁口径；SQL、
   Python、notebook、DuckDB、shell 等可复核手段只受 source、scope、只读、
   validation、delivery 和 evidence boundary 管理。
2. 必须实读本机 Codex 已安装的 Data Analytics 插件技能与 reference，至少包括
   `index`、`user-context`、`analyze-data-quality`、`validate-data`、
   `jupyter-notebooks`、`metric-diagnostics`、`visualize-data`、
   `build-dashboard`、`build-report`，吃透 `index -> preflight ->
   focused skill -> source verification -> validation -> delivery` 技术路线，
   再按经侦资金分析领域化落地；不得凭印象、摘要或旧讨论替代对本机插件实现的研究。
   若发现现有经侦领域方法覆盖不足，例如团伙关联、IP/MAC/设备、开户地址/联系、
   现金同存同取、资产消费、理财证券、境外转移、空壳/中转/汇聚账户、重复/换卡/
   同事实等，必须扩展领域 reference、focused skill、能力注册、workbench 指引或
   source helper，而不是用一句“后续补充”跳过。
3. 15 项 Data Analytics 等价软边界是发布级基准，不是硬路由或硬停止：preflight envelope、semantic layer as map、
   source-of-truth selection、source guardrail、live/source-backed verification、
   SQL/Python/notebook allowed when useful、data quality checks、validation state、
   delivery contract、render/execute QA、audience language、index/navigator not
   commander、focused workflow ownership、report completion gate、scoped state/memory。
4. 14.2 借鉴机制落地矩阵必须逐项核验。最低要求是达到成熟插件同等机制效果；
   更高目标是在经侦资金案件场景下超过借鉴对象。CodeGraph 对应 casegraph/
   fundgraph 上下文；Data Analytics 对应多 skill、source guardrail、delivery
   contract；Impeccable 对应术语、反模式、critique；Superpowers 对应复杂全案
   计划/复核；oh-my-codex 只借 doctor/runtime 状态治理，不污染系统 Codex。
   每项必须写出 mature principle、Analytix 程序落点、parity/gap、经侦增强点、
   证据产物和 release blocker 判断。
5. 必须实读本机 `Investment Banking` 和 `Public Equity Investing` 插件关键机制：
   IB 的 invocation gate、plugin routing playbook、lead skill handoff、internal support
   和 deliverable intake；PE 的 final deliverable framework、support-layer routing
   contract、PM judgment heuristics、source/research support standard。Analytix 必须吸收
   lead workflow owner、support-layer hidden、hero deliverable first、专业 judgment
   layer、reader-ready final handoff；不得把 MCP/card/workbench/support text 当用户最终研判。
6. 施工不是追求脚本分数或旧式对比实验。测试必须发现偏离成熟插件、偏离蓝本、
   偏离真实用户研判目标的问题，并把提升落到 skill、MCP、runtime、doctor、
   功能测试、release guard、前门验收和发布产物。
7. 发布授权不取消隔离边界：仍禁止修改系统 Codex、系统 Codex 全局 home/config/cache、系统
   hooks/MCP/config/cache；禁止把 eval fixture、golden/oracle、旧 evidence、
   真实案件固定答案写入 production resources；禁止跨案事实记忆。
8. commit/push/tag/admin-console 发布由施工 Agent 自主决定，但必须先满足：
   doctor/功能测试/release audit、runtime cache/case-project source、Hub package、真实前门
   功能闭环、当前版本/HEAD/tag/package hash 身份一致。
9. 遇到认证、网络、权限、admin-console 登录态、Hub 后端、真实 case-project source 或
   runtime cache 问题，先自行诊断解决；只有需要用户人工登录、二次验证或外部
   账号操作时，才记录真实 blocker。
10. 用户交付层 P0 必须优先于发布：事实正确但表达像审计清单、工具口径说明或
   内部流程复述，仍判失败。Data Analytics 的 source、validation 和 delivery
   细节应保存在 metadata/source notes/artifact 中，用户正文必须 answer-first、
   经侦材料化、表格/图谱服务研判结论。不得把“必须保留小标题”“口径与来源边界”
   “direct / candidate”等内部合同当成用户答案。
11. 专业判断层 P0 必须优先于发布：事实正确但只给数字/口径/边界，缺少经侦判断、
   大额交易表、异常特征、关系/角色线索、下游追踪和补证动作，仍判失败。学习 PE
   的 PM judgment，但转写为经侦判断：资金性质、角色关系、异常点、证据缺口、下一步。
12. 成熟插件结构化重构 P0 必须优先于发布：root/index/`funds_investigate`
   只做入口、navigator 和 support；`answer_draft`、card title、workflow、
   support facts 不得成为普通 agent-readable 成稿主体。Pair Amount 必须由
   `pair-amount-investigation` 或等价 focused owner 成稿；`quick-fact` 只保留
   低风险单点事实和排行。
13. 经侦专家交付层 P0 必须优先于发布：事实正确、口径正确、内部词不外泄仍不等于
   合格。最终输出必须有案件研判 spine、研判材料 plan、证据成熟度语言和侦查判断点；
   需要表格、图谱、报告、补证清单时必须作为读者主交付，而不是工具摘要。学习
   Data Analytics 的 report spine、IB 的 memo plan、PE 的 PM judgment，但全部转写为
   公安经侦业务语言。
14. Analytix 软件工作流智能层 P0 必须优先于发布：插件要让 Codex 更懂 Analytix
   软件全过程，包括当前案件、清洗表、analysis 索引、导出字段、图谱、报告、
   附件和多轮继续推进。不能模板化输出，也不能把每个问题都压成事实卡。

#### 15.6.2 当前新线程 `/goal` 粘贴稿

```text
/goal
目标：作为唯一施工线程，在 /Users/sun/Projects/analytix 按 top-pluginization-plan.md 与 blueprint-execution-plan.md 把 Analytix 涉案资金研判插件推进到“Data Analytics 母版 + 经侦领域化”的商业级闭环。你已获全权授权：可自主决定修改、测试、commit、push、tag、生成 Hub/admin-console 包，并在所有 blocker 清零后通过 https://analytix.top/admin-console 发布最新版；禁止伪造发布、绕过认证、用本地 remount 冒充官网发布。旧线程只作背景，不继承旧打分、旧对比实验、旧工具禁令或未验证结论。

基准路线：必须实读本机已安装 Data Analytics 的 index/user-context/analyze-data-quality/validate-data/jupyter-notebooks/metric-diagnostics/visualize-data/build-dashboard/build-report 等 focused skills，吃透 source + validation + delivery 技术路线后经侦领域化；经侦覆盖不足就扩展领域方法、skill、workbench 或 source helper，该大改就大改。落地 index router -> case_source_envelope -> focused workflow -> live/source-backed verification -> validation state -> delivery contract。Data Analytics 没封锁的 SQL/Python/notebook/DuckDB/shell，Analytix 不自创封锁；这些手段只受当前案件、清洗/analysis scope、只读、限行、purpose、validation、delivery、artifact/evidence boundary 管理。只阻断无来源强答、弱来源冒充、未验证计算、假交付、内部实现外泄、跨案事实污染和固定样例污染。

新增基准：必须实读本机 Investment Banking 与 Public Equity Investing 插件，重点研究 IB 的 invocation gate、routing playbook、lead skill handoff、internal support、deliverable intake，以及 PE 的 final deliverable framework、support-layer routing contract、PM judgment heuristics、research support standard。吸收为 Analytix 经侦化机制：root/index/funds_investigate 只做入口和 navigator；focused skill 才是金额核验、主体画像、资金穿透、全案研判、图表/报告的 lead owner；MCP/card/workbench/ledger 只做 support layer 并隐藏内部字段；用户先看到经侦 hero deliverable 和专业判断，不看 workflow、case_id、supported seed、delivery_state、handoff JSON。必须新增或落实 `pair-amount-investigation` 等价金额核验 owner，`quick-fact` 退回低风险单点事实和排行。

先读：蓝本、执行方案全文尤其 0.0、0.6、0.7、4、10、11、13、14、15.6.1、蓝本 14.2 借鉴矩阵、registry、metadata、tool-availability、anti-patterns、runtime-boundary、focused skills、MCP tool schema/runtime、doctor、runtime-cache、release/package 脚本。运行 git status --short 冻结现场，保护用户改动，不做无关回滚。

先审计再施工：输出 drift 表，主动找硬路由、硬停止、机械总指挥、追分导向、假通过、旧 evidence、旧工具封锁、MCP 固定答案、golden/oracle 泄漏、跨案记忆、弱来源冒充、二次污染、raw rows/大 JSON/重复 JSON/转义 JSON 外泄、support layer 被模型复述、假交付、当前案件同步失败、版本/runtime/cache 不一致。凡是“只能走某个 MCP 读取 DuckDB 并返回最终答案”的 production 路线，直接从 tool discovery、tool schema、skill reference、answer contract 和普通任务中删除或禁用；可复用部分只能重建为 source map、query guidance、workbench template、validator、artifact bridge 或 provenance carrier。MCP 不是总指挥，casegraph/fundgraph 是上下文地图。

最新 P0 事故必须优先闭环：真实 analytixagent 前门首跳出现 case-project source API `current case project is not set`，随后 Agent 列目录、搜索本地 DuckDB、尝试历史/测试 case_id 并触发 `case not found`，工具调用膨胀且来源不稳。analytix 案件项目会通过工作区上下文进入 Agent，必须把案件项目工作区绑定到 `.analytix/case-project.json`、analytix runtime `_analytix.workspaceRealPath`、完整冻结案件授权和 `analytix_funds`；同步失败时只返回 Case Source Blocker，不得扫描 `/Users`、Downloads、case 目录或从 fixture/历史输出猜 case_id。

按蓝本完整落地 Data Analytics 等价机制和 14.2 借鉴矩阵：root/index/funds_investigate 只做入口、navigator、capability discovery；focused skills 拥有 workflow 与 completion gate；默认 source-backed 能力层覆盖 current case、scope、casegraph、rank、profile、trace、fundgraph、hypothesis、quality、validation、navigator；低层 raw/debug/write/export/full discovery 只在开发诊断、显式授权、专项测试或正式 report lane 条件可见。14.2 每项必须写 mature principle、Analytix 程序落点、parity/gap、经侦增强、证据产物、release blocker。Impeccable 的术语、反模式、critique 要覆盖普通问答、Lab、图谱、报告；CodeGraph 落到 casegraph/fundgraph；Superpowers 只进复杂全案/报告/重大 QA；oh-my-codex 只借 doctor/runtime 状态治理，不污染系统 Codex。

P0 必修：修 Pair Amount / Amount Challenge 错误族，不写死案例。对“A 转给 B 多少钱”“金额范围对吗”“两个金额范围哪个对”，必须由 `pair-amount-investigation` lead owner 或等价 focused workflow 成稿，核对 source-of-truth、holder/account scope、counterparty grain、raw/effective/dedup、时间窗、同事实/换卡/重复候选；rank_counterparties 和 funds_investigate 只能给排行/导航/候选支持，不能单独最终作答。语义工具不足时进 case-workbench targeted 聚合，并返回 validation state、source boundary、delivery state。占位/空值/`查无信息`/unknown/无/null/明显非唯一交易号不能单独作为去重主键；有效交易号也必须结合时间、金额、方向、余额、对手账号、解析对手户名、摘要/备注、成功状态等复合键复核。答案必须作为两方金额核验材料输出：解释 raw 明细金额、high-confidence same-fact effective 金额、可选本金/手续费口径、duplicate/unsupported 金额、差异原因、账户/时间集中、重复/换卡风险、关系/角色线索和下一步核验，而不是只答一个数字。

最新用户交付层 P0 必修：事实正确但表达像审计清单、工具说明或内部流程复述仍判失败。清理普通用户最终回答、MCP/card agent-readable、图谱、主体画像、报告中的工具预检句、同名聚合内部标题、核心账号内部标题、来源/数据审计标题、图谱依据表标题、英文对象画像标题、`direct/candidate` 原词、候选数量机械标签以及“必须保留/不要合并或改名”等内部合同。审计层保留到 `_meta`、ledger、doctor、eval、artifact；用户正文改成公安经侦报告口吻：结论先行、事实表格、研判意义、异常特征、资金去向、必要边界短写和核查建议。重点修 destination-diagnostic-runtime、agent-output-compiler、fund-flow-graph-runtime、graph-visualization/SKILL、subject-dossier/SKILL、report/visual 交付合同；新增 `user_delivery_contract` 正负测试和真机多轮验收。

新增专业判断层 P0 必修：不能把“能查到金额”误判为“能研判”。仿照 PE 的 PM judgment 和 IB 的 client-ready delivery，经侦输出必须回答“这笔/这组资金有什么案件意义、异常在哪里、还不能证明什么、下一步查什么”。Pair Amount 最小合格输出：结论句、期间、笔数/金额、主要集中日/集中账户、大额交易 Top 表、竞争金额业务解释、重复/换卡/同事实风险、异常特征扫描、关系/角色或资金目的线索、下游追踪建议。主体研判、流向图、全案报告同样必须有事实表、研判意见和补证动作；只说“两个统计范围不同不能相加”或只列数字，判 fail。新增 `professional_judgment_contract`、`support_layer_hidden_contract`、`pair_amount_mini_investigation_smoke`、`hero_delivery_contract`。

新增经侦专家交付层 P0 必修：把 Data Analytics 的 report spine、IB 的 memo plan、PE 的 PM judgment 转成 Analytix 的案件研判 spine、研判材料 plan、证据成熟度和侦查判断点。所有实质输出必须能回答：查的是什么、结论是什么、依据哪些流水/账户/链路、异常在哪里、这对案件有什么意义、还不能认定什么、下一步调什么材料。新增或强化 `delivery-qc`，检查 Pair Amount、主体/账户研判、资金流向图、全案报告、附件清单是否达到公安经侦材料口吻。用户可见表达使用“已有流水支持/高可信支持/线索/需复核/暂不能认定/可形成材料候选”，不得使用 screen-grade/support layer/delivery_state/direct/candidate 等工程或金融插件原词。

新增 Analytix 软件工作流智能层 P0 必修：复盘无插件旧线程的成功行为，但不得把旧案件姓名、金额、固定结论写入 production reference。建立 `analytix_workflow_context` 或等价共享 reference，让 Codex 默认理解案件中心、当前案件同步、清洗/analysis 表、清洗导出字段、报告目录、附件目录、Mermaid/PNG/JPG 图谱和多轮继续推进。用户要明细时走清洗导出口径，用户要图时通过可用 Analytix/Codex 交付面生成并检查真实图像/表格 artifact；无交付面时写交付缺口，不得把聊天图描述冒充文件。用户要继续完善报告时保留既有报告结构并局部补写/核算，用户要深挖时按侦查假设验证链路。新增 `analytix_workflow_context_contract`、`non_template_output_contract`、`cleaned_export_contract`、`report_continuation_contract`、`visual_artifact_contract`。

输出与交付：Answer/Lab/Dossier/Full Case/Visual Evidence/Report 必须给研判结论、事实、线索状态、证据边界、缺口和下一步；不得输出蓝本、执行方案、/goal、doctor、内部评测标签、diagnostic label、report gate、write_blocked、内部 id、raw rows、大 JSON。answer_card_complete/max_additional_tools 只代表本轮预算，不压制继续追一层、改口径重算、否定性搜索、开放深挖。图表/矩阵/dashboard 只作视觉证据；报告、图表、notebook、附件包必须有真实 artifact、render/execute QA 或 blocker。

验证不要跑旧式对比实验，不追脚本高分。按执行方案跑静态检查、doctor、runtime cache、release identity、CLI/direct MCP、functional-eval 或迁移后的功能 runner、旧线程行为 replay、真实 rollout 审计、输出污染阻断、Pair Amount、workbench success/gap、真机功能闭环。必须增加 case-project source 冷启动/上次案件恢复/切换案件/新建 Agent 会话回归；必须增加 Pair Amount 去重键安全回归，断言占位交易号不会合并不同交易，错误 effective amount 被拒绝，排行金额不能当最终 Pair Amount；必须用 `agent-thread-locator` / `agent-thread-audit` 或等价工具定位真实 thread/deeplink，统计 tool calls、MCP payload bytes、模型可见 JSON/重复 JSON/internal leak 和最终回答质量。功能闭环产物写入 output/analytix-fund-analysis/functional-closure/<timestamp>/，包含 drift、MCP 去中心化审计、case_source_envelope、pair amount、workbench、doctor、runtime cache、frontdoor functional、replay、agent-thread-audit、output leak、case sync、manual UI、release identity、changed files、validation commands。Hub package 不得包含旧 evidence、eval-only、golden/oracle；release audit 必须拒绝旧版本、旧工具封锁口径、无 runtime cache、无真实前门、只跑 CLI/direct MCP 的产物。

发布授权不取消隔离边界：禁止修改系统 Codex、系统 Codex 全局 home/config/cache、系统 hooks/MCP/config/cache；禁止跨案事实记忆；禁止把真实案件固定答案写入 production resources。遇到认证、网络、权限、登录态、Hub 后端、case-project source、runtime cache 问题，先自行诊断解决；只有需要用户人工登录或二次验证时才记录 blocker。

完成标准：蓝本/执行方案/skill/metadata/registry/runtime/doctor/功能测试不打架；Data Analytics source+validation+delivery 等价机制落地；14.2 借鉴矩阵达标并有经侦增强；root/index/funds_investigate/card/workbench/support layer 不再替 Codex 做任务理解、路径选择和最终结论；`answer_draft` 不再作为普通 agent-readable 主体；Pair Amount 由独立 focused owner 成稿且不再因重复/换卡/排行误算；`delivery-qc` 或等价机制能阻断事实正确但无案件研判 spine、无证据成熟度、无异常特征、无侦查动作的低质回答；`analytix_workflow_context` 或等价机制能证明 Codex 理解 Analytix 当前案件、清洗导出、图谱、报告、附件和多轮继续推进；用户交付层不再外露审计合同且达到公安经侦报告口吻；Case Workbench 自由分析且受控可审计；focused skills 各自有 workflow；当前案件同步可回放；真实前门功能任务给出正确 source-backed 经侦研判；release identity 一致且无发布级 blocker。完成后输出 commit、tag、push、admin-console 版本、包 hash、验证命令、功能闭环目录和剩余风险。
```

### 15.7 旧长提示词清理说明

旧历史长稿正文已从执行方案中删除。删除原因：它包含旧授权边界、旧对比实验/
追分口径，以及可能与 15.6 发布版合同冲突的可粘贴内容。当前施工
只能使用 15.6.2；旧稿如需审计，使用 git 历史，不在当前执行方案中保留。


### Conditional Release Blocker Notes

- Conditional release blocker: Pair Amount may ship only when focused owner routing, support-layer hiding, and final Chinese amount review all pass functional-eval.
- Conditional release blocker: Visual/report artifacts may ship only when generated surfaces are inspected and delivery gaps are explicit.
- Conditional release blocker: Current-case workflow continuation may ship only when case sync, cleaned export, report patching, and attachment handoff remain source-backed.
