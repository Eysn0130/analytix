# Analytix 涉案资金研判插件系统蓝本

## 2026-06-30 analytix 案件项目适配权威说明

本蓝本已从旧 `Analytix_new` 形态迁移到当前 `/Users/sun/Projects/analytix/plugins/analytix-fund-analysis`。旧项目目录只可作为历史来源和对照材料，不再作为开发、发布、运行、证据、测试或案件定位依据。

当前插件的唯一产品归属是 `analytix` 桌面应用；运行时只能服务当前 analytix 案件项目。案件来源链路必须是：

```text
analytix 案件项目工作区
-> <case-project>/.analytix/case-project.json
-> ~/Library/Application Support/analytix/data-analysis/cases/<case_id>/case.duckdb
-> analytix runtime 注入 _analytix.workspaceRealPath/threadId/turnId
-> analytix_funds MCP / Controlled Case Workbench
```

当前生产桌面模型工具调用只由 `packages/runtime-go` 执行；
`packages/runtime` 只保留公共 contracts/config/telemetry 和启动 Go
`runtime-server` 的 TypeScript launcher。Go Harness 必须在调用
`mcp__analytix_funds__*` 前注入经 Core 授权的 `_analytix` 上下文。若只有
MCP 进程能识别案件，而真实 Go 会话没有注入 workspaceRoot，Agent
会在已进入案件项目的情况下误判“当前案件来源尚未就绪”，这是
P0 级上下文断链。

因此，旧文档中所有“后端激活案件”“全局 active-case”“上次打开案件同步到 backend active-case”的表述，在当前 analytix 中均修正为“当前会话所在案件项目工作区绑定”。MCP 可以读取显式传入的 `case_id`，但必须与当前案件项目绑定一致；不一致时返回跨案 blocker，不能静默取其他案件数据。后端 API 不可达时，只要案件项目配置和 DuckDB 可用，就不得把它解释为“案件未就绪”；只能记录 warning，并继续通过本地 DuckDB 只读事实层和 Workbench 取数。

插件必须保留 Data Analytics 式开放分析路径：语义 MCP 优先，但不是 MCP-only。语义工具满足不了、用户挑战口径、结果冲突或需要自定义核算时，必须像 `crystaldba/postgres-mcp` 吸收的数据库现场诊断模式一样，让 Agent 能在受控边界内看见当前案件 DuckDB 的 schema、表行数、字段 profile、查询计划、样本预览、只读 SQL 结果、执行历史和审计证据。该路径只能访问当前案件 DuckDB 的清洗表和分析索引，必须只读、限行、有 purpose、可回放、有 evidence boundary，不得扫描本地目录、读取旧 evidence、跨案 join 或访问外部文件。

文档中的旧事故、旧命令、旧测试产物和旧发布说明仅保留为研发背景。
平台品牌、唯一 Go Harness、Privacy Layer 与 Core/Plugin 边界以
`docs/analytix/specs/11-agent-platform-brand-and-architecture.md` 和 accepted
`openspec/specs/agent-platform-foundation/spec.md` 为准；Funds 域细节再以本蓝本与
当前插件代码/门禁为准。`/Users/sun/Projects/analytix/重构升级方案.md` 仅是
历史决策账本，不再是当前控制权威。

本文档定义 `Analytix 涉案资金研判` 插件的完整系统蓝本：产品目标、插件身份、能力体系、工程架构、运行链路、证据协议、资金链路穿透模型、报告交付、质量治理、边界隔离、施工控制、Reference 映射、外部借鉴内化和商业成熟完成定义。

它不是教程、讨论记录、调研纪要、发布记录或面向普通用户的说明书。它是 Funds
域的完整参考蓝本，但不是 Analytix 平台架构、施工调度、Git 提交或发布
验收的唯一 authority；这些仍服从当前 `AGENTS.md`、accepted specs、
authorized OpenSpec 和 fresh evidence。

本文历史段落中的 `analytixagent` / `analytix-agent` 只表示 Analytix 内嵌
agent app-server 或 eval 二进制的内部组件名，不是公开品牌、产品类别、第二
runtime 或独立发布身份。

未使用插件时，Codex 借助通用 SQL/Python/notebook 探索案件数据、反复查询、反复修正，已经证明一种有价值的侦查式探索能力：它可以从数据结构、统计口径、异常线索、资金路径、报告表达和用户追问中不断校正判断，发现预设流程之外的问题。插件的目标不是复制原生探索路径，也不是给 Codex 新增 Data Analytics 没有的工具禁令，而是以官方 Data Analytics 的商业级插件形态为母版，把这种“直接查、自由想、反复核、继续追”的能力，产品化为 Analytix 边界内可复核、可审计、可交付、可评估的专业资金研判能力：先用轻量案件上下文和语义地图定位 source-of-truth，再由 Codex 自主选择最小充分的 source-backed 路径；路径可以是 source helper/MCP、casegraph/fundgraph、受控 SQL/Python/notebook/workbench、图表/报告/附件交付或后置 critique。MCP 只是承载可审计事实能力的一种接口，不是替 Codex 决策的总指挥。

```text
产品主线：Data Analytics 式入口 -> 案件 source preflight -> focused workflow -> live/source-backed verification -> delivery / critique
工程主线：Plugin package -> Focused skills -> Source map / casegraph -> Source adapters / Workbench -> Evidence / Artifact -> Validation / Delivery
治理主线：轻量默认 -> 风险触发升级 -> 反模式 -> doctor / 功能闭环 -> Hub release -> 完成定义
```

## 0.0 方向修正：让 Codex 更会想

2026-06-03 的真实线程审计确认：本蓝本原目标层是正确的，但部分工程契约把“可审计事实层”误实现为“统一前门接管 Codex 思考”。这种实现会让插件在安全上更保守，却在真实案件问答中表现为机械、固定链路、说明书式输出和开放深挖能力下降。

本蓝本的最高产品原则修正为：

```text
只锁事实来源，不锁思考路径。
```

插件必须锁住事实来源、验证状态和交付真实性：

- 必需案件来源不可用时，不输出报告级确定结论；可选 enrichment 缺失时继续分析但标注 gap。
- 语义层、casegraph/fundgraph、scope map、dashboard metadata、bounded rows 和局部 preview 只是地图、样例或线索，不能替代 live source verification。
- SQL、Python、notebook 或自定义计算可以作为受控分析手段，但只有带有当前案件、只读、清洗表/分析索引、限行、purpose、验证状态和 evidence boundary 的结果才能支撑用户可见事实。
- 生产研判事实使用 `fc_*_norm`、`analysis_*` 和 backend 编译事实卡；`fc_*_raw`、原始文件或未复核来源只可作为覆盖/质量/诊断线索，不能冒充普通研判事实。
- Agent 不把 candidate account、same-fact candidate、现金断点、缺失对手方、资产/理财线索写成确认事实。
- 报告级金额、链路、法律敏感 claim、附件清单和 Mermaid supported edge 必须可复核。

插件不得锁住：

- Codex 自己选择先查 scope、rank、holder、account、counterparty、trace、graph、duplicate、quality 还是 hypothesis。
- Codex 自己提出侦查假设、反证路径、改口径重算、否定性搜索和继续追一层。
- Codex 根据用户追问重新进入新工具、新对象、新时间窗或新口径。
- Codex 在语义工具不足时提出并执行受控的自定义口径、SQL/notebook 或可回放分析请求。
- Codex 将多个工具返回的事实组织成自然、简洁、有判断力的中文研判。

因此，`funds_investigate` 不能再被定义为替 Codex 思考的总指挥。它只能是自然语言快捷入口、router hint 和轻量 navigator。默认生产工具面必须从“少量前门工具”改为“可审计语义事实工具箱”：让 Codex 能看见并自主选择高价值事实工具，同时用事实来源、输出后置 critique、Evidence Ledger 和 Claim Verifier 管住错误事实和越界表达。

外部借鉴也按此原则重新解释：

| 来源 | 真正增强 Codex 的机制 | Analytix 正确转译 |
| --- | --- | --- |
| codegraph | 给 Agent 预索引图谱，让它更会看上下文。 | `casegraph/fundgraph` 必须成为可查询事实底座，而不是固定事实卡。 |
| impeccable | 给 Agent 专业词汇、反模式和 critique，而不是替它设计。 | `/analytix` 命令族应成为经侦研判词汇和检查器，不是隐藏路由表。 |
| superpowers | 复杂工程任务才进入计划、执行、复核。 | 全案报告/复杂多主体任务可用 Plan DAG；普通问数和追一层不能被流程化压住。 |
| oh-my-codex | 增强状态、日志、doctor，不替代 Codex execution engine。 | 借鉴 Analytix-owned case journal、doctor 和运行状态；不接管系统 Codex，也不让插件总控思考。 |

后续任何施工若让普通研判更像模板、更少工具选择、更少假设、更少反证，即使 doctor 通过，也视为产品退化。

### 0.0.1 Data Analytics-first 母版重定向

本蓝本从本节开始不再以 Analytix 现有 frontdoor、某个 MCP 工具或某条固定资金链路为中心。Analytix 是交付给 Codex 使用的商业级数据分析插件，母版是官方 Data Analytics，而不是现有 MCP 失败路径的修补版。

Data Analytics 的可复制机制不是“更多工具控制模型”，而是低上下文脚手架。后续施工必须实读本机 Codex 已安装的 Data Analytics 插件技能与 reference，吃透其 `index -> preflight -> focused skill -> source verification -> validation -> delivery` 技术路线后再经侦领域化；不得凭印象、摘要或旧讨论替代对本机插件实现的研究。

| Data Analytics 母版机制 | Analytix 正确等价 | 禁止误读 |
| --- | --- | --- |
| `index` 只选择 focused skill。 | `index` / root 只做入口、能力发现和轻量路由。 | 不在 index 中跑全案、报告或固定链路。 |
| `user-context` preflight 先读取紧凑 envelope。 | `case-context` 读取 current case project、cleaned/analysis coverage、casegraph/fundgraph、source gap 和 delivery obligation。 | preflight 不是硬路由，也不是每次全量审计。 |
| semantic layer 是地图。 | casegraph、fundgraph、scope map、schema、command metadata 只帮助找数和理解口径。 | 语义地图、样本行、dashboard metadata 不得写成结论。 |
| focused skills 拥有 workflow 和 completion gate。 | quick-fact、data-quality、dossier、fund-tracing、lab、full-case、visual、report、workbench、critique 各自拥有任务面。 | skill 不等于 MCP tool 外壳，也不复制整份蓝本。 |
| SQL/Python/notebook 是正常分析路径。 | 明确自定义口径或语义工具不足时，进入当前案件、只读、清洗/analysis scope、限行、purpose、validation、delivery 约束的 Workbench。 | 不把 SQL/Python/DuckDB/shell 写成 blanket forbidden。 |
| validation 是风险触发升级。 | 重复/换卡、口径冲突、join 风险、报告 claim、图表交付、异常金额触发 data-quality / validate / critique。 | 普通 Top/ranking 不默认跑全量 QA，也不因高风险缺口直接拒答。 |
| delivery contract 管交付质量。 | 报告、图表、表格、notebook、证据包只有在可用交付面生成并检查后才可宣称交付；否则必须明确 blocker。 | 聊天摘要不能冒充交付物。 |

因此，Analytix 的商业成熟路线是：

```text
Data Analytics 式多 skill 产品骨架
+ 轻量 case source preflight / case semantic map
+ 可审计资金事实工具箱
+ 受控 SQL/Python/notebook Workbench
+ Impeccable 式命令、反模式、critique
+ CodeGraph 式 casegraph/fundgraph 上下文
+ Superpowers 式复杂全案计划/复核
+ oh-my-codex 式 Analytix-owned doctor/runtime 状态治理
```

性能和 token 原则同样按 Data Analytics：默认只加载短 skill、紧凑 envelope 和一个最小充分工具；只有当工具返回 `needs_review`、用户挑战口径、结果将进入报告/图表/附件、或语义工具无法回答明确任务时，才升级到 quality、workbench、notebook、claim-review 或 full-case。商业级插件不是让每个问题都重算全案，而是让 Codex 在最小上下文里知道何时可以答、何时必须验证、何时应交付 artifact、何时必须标注 gap。

#### 0.0.1.1 MCP-first 语义事实层与 Workbench 升级序

Analytix 对齐 Data Analytics，不等于让 Codex 绕过插件先裸跑 DuckDB/SQL/Python/notebook。Data Analytics 面向多数据源，所以用 source connector、semantic layer、notebook 和 report contract 组合；Analytix 面向当前案件 DuckDB，`analytix_funds` MCP 应是 Analytix 官方 source connector + semantic map + governed executor + validator 的合体。普通案件事实题默认应先通过 MCP 语义事实层读取当前案件，而不是让模型先从 shell、文件系统、历史输出或本地数据库路径自行摸索。

正确升级序：

```text
focused skill 判断用户任务
-> case_source_envelope / current-case source-of-truth
-> MCP semantic fact layer 读取当前案件清洗表/analysis index
-> Evidence Ledger / validation state 合并候选事实
-> 若语义事实不足、冲突、被用户挑战或用户要求自定义口径
   才进入 Controlled Case Workbench / SQL / Python / notebook
-> focused skill 用经侦业务语言成稿，MCP/Workbench 不拥有最终答复
```

因此，本蓝本同时否定两种错误路线：

- `MCP-only`：把某个 MCP、`funds_investigate`、rank 或 answer card 写成唯一前门、唯一事实源或最终答案 owner。
- `raw-analysis-first`：把 Data Analytics 的 SQL/Python/notebook 能力误读成普通案件事实默认先裸查 DuckDB、shell、工作目录或历史文件。

SQL/Python/notebook/DuckDB 是受控升级能力，不是被禁能力，也不是默认首跳。它们必须通过当前案件、只读、清洗/analysis scope、限行、purpose、validation、artifact/evidence boundary 和审计记录进入结论。`shell` 主要用于开发、打包、artifact 检查、可视化/报告 QA 和插件工程验证；普通案件事实不应以 shell 搜索本地案件目录、历史会话或旧 evidence 作为 source-of-truth。

这正是三款成熟插件的共同机制：Data Analytics 先做 source preflight 和 semantic map，但仍要求 live source verification；Investment Banking 和 Public Equity Investing 让 router 只 admission/handoff，support 层只供证据和执行，专业 lead skill 才负责判断和交付。Analytix 的 MCP 应成为经侦事实层的“可靠取数器和验证器”，不是机械总指挥，也不是可被模型随意绕过的装饰工具。

#### 0.0.1.2 顶级经侦研判闭环与续调门槛

Analytix 的目标不是让 Agent 更会复述工具事实，也不是让 MCP 限制 SQL/Python/notebook/DuckDB，而是把 Codex 的自由探索能力产品化为“公安经侦专家式”研判闭环：先确认当前案件 source-of-truth，再把语义地图、MCP 事实、受控 Workbench、图谱、报告和复核统一到同一 Evidence Ledger；在当前案件数据已足以回答时直接形成研判，在当前案件数据确实不足时才提出续调动作。

专业闭环必须同时满足：

- **source preflight 已完成**：确认 current case project、清洗表/analysis 索引覆盖、时间范围、字段缺失、来源缺口、报告/图表/附件交付要求；可选 enrichment 缺失只标注 gap，不阻断当前可答问题。
- **semantic layer 只作地图**：schema、casegraph、fundgraph、rank、profile、scope、dashboard metadata 负责定位对象、字段、候选链路和风险点，不能替代 live verification。
- **live verification 已落地**：金额、笔数、账户、对手方、资金边、资产/理财/现金线索等重要 claim 必须来自 MCP 事实层或 Controlled Workbench 的当前案件只读查询，并进入同一 Evidence Ledger，带 scope、grain、time_window、dedup、source_hash / query_id 和 validation_state。
- **当前案件数据先分析充分**：一跳金额、主体画像、资金流向、全案报告等任务，必须先在当前案件 DuckDB 的 `fc_*_norm`、`analysis_*`、casegraph/fundgraph 可得范围内核查相关账户（列明账号）、入账环节、出账去向、集中日期、大额交易、重复/换卡、同事实候选、缺失对手方、现金/理财/证券/基金/资产消费等线索；不能只看 rank、样本行或固定事实卡就下结论。
- **续调建议后置**：只有当当前案件清洗表和 analysis 索引已经分析到数据边界，且下一层资金穿透依赖外部材料时，才给续调建议。续调清单必须列明对象、账号/户名、期间、材料类型、验证目的和当前数据为什么不能证明；不得用泛化的“建议进一步核查”替代具体补证动作。
- **专业判断必须成稿**：最终回答要把事实转成经侦判断，说明资金意义、异常特征、证据成熟度、暂不能认定事项和下一步证明路径；不能把 SQL、MCP、notebook、support layer 或测试口径当作用户正文结构。

因此，SQL/notebook 的正确地位是“可回放的 live verification 能力”。它们既不是普通问题的无脑首选，也不是危险到必须禁止的路径；当语义事实不足、用户挑战口径、需要自定义计算、需要导出明细、需要报告/图表可回放时，进入当前案件、只读、清洗/analysis scope、限行、purpose、validation 和 artifact/evidence boundary 约束下的 Workbench，才是成熟插件等价路径。

### 0.0.2 MCP 去中心化与 DuckDB 工具大砍原则

如果某条技术路线要求用户问题“只能走某个 MCP”，或要求某个 MCP 直接读取 DuckDB、套固定聚合、返回最终研判答案，那么这条路线与 Data Analytics 母版冲突，应当大砍。Data Analytics 的强项不是把 Agent 绑到一个数据工具上，而是让 Agent 先理解问题、选择 source-of-truth、验证计算、再按交付面输出。

Analytix 不是删除所有 MCP，而是直接砍掉错误 MCP 路线：凡是替 Codex 理解任务、选择路径、固定 DuckDB 口径或输出最终研判答案的 MCP，必须从 production 默认面删除或禁用；只有通过审计证明只是来源、执行、验证、artifact/provenance 承载层的 MCP，才允许保留。

允许保留的 MCP 角色只有：

| MCP 正确角色 | 含义 | 不再承担 |
| --- | --- | --- |
| Source adapter | 解析当前案件、清洗表/analysis index、scope、schema、casegraph/fundgraph 和来源状态。 | 替 Codex 判断用户到底要查什么。 |
| Governed executor | 为 workbench / notebook / SQL 提供当前案件、只读、限行、审计、artifact 和 provenance。 | 让普通用户只能走一个封装好的 DuckDB 查询工具。 |
| Semantic map helper | 给低成本 rank/profile/trace/quality 线索和候选 source。 | 把 rank 行、样本行、bounded rows 直接写成最终 claim。 |
| Validator | 对金额、重复/换卡、join 粒度、路径、Mermaid、报告 claim 和法律敏感表达做复核。 | 在分析前替模型预设固定结论或固定流程。 |
| Delivery / artifact bridge | 保存报告、图表、notebook、证据包、ledger 和 QA 状态。 | 用聊天摘要冒充 artifact，或用 MCP 内部 JSON 作为用户交付物。 |

因此，后续工程必须按“先砍再重建”的顺序处理 MCP：

- 会直接生成最终中文研判结论的 MCP：从 production tool discovery、tool schema、skill reference、answer contract 和普通 eval 中删除或禁用；若确有价值，只能重建为 focused skill 的写作/交付逻辑，事实仍来自 source-backed path。
- 会把 DuckDB 查询写死成单一业务口径的 MCP：从 production 默认面删除；只允许把可复用 SQL 思路迁移为 reference 指引、query guidance 或 workbench template，且必须允许 Codex 按用户问题改口径。
- 会返回大 JSON/raw rows/support envelope 的 MCP：从模型可见默认面删除；只能进入 artifact / `_meta` / ledger，不作为普通用户或模型上下文的事实来源。普通 tool content 必须是短事实摘录、事实表或业务化边界说明，不得把 `support_only`、`case_id`、`workflow`、`answer_card_complete`、`delivery_state` 等内部字段交给模型。
- 会把 `rank_counterparties`、Top、bounded preview 当最终事实的 MCP：删除其最终 claim authority；只有在输出 `ranking_only`、`needs_review` 或 `candidate` 且不能控制 Pair Amount 时，才可作为低成本候选 helper 保留。
- eval-only、golden、diagnostic、report-gate、write-blocked 类型 MCP 输出：从 production resources、skill references、用户输出和普通 tool discovery 中删除，只保留在 eval/dev/release audit 隔离面。

保留 MCP 的审计五问：

1. 它是否只提供 source map、schema、scope、provenance、执行或验证？
2. 它是否拒绝输出最终中文研判答案？
3. 它的结果是否能进入同一个 Evidence Ledger，而不是形成第二套事实源？
4. 它是否允许 Codex/Workbench 按用户问题改口径？
5. 它是否在 tool discovery、skill、eval 和 answer contract 中明确不是总指挥？

任一答案为否，默认直接砍出 production 路径，再决定是否以 reference、workbench template、validator 或 artifact bridge 的形式重建。

“Data Analytics 母版 + 经侦领域化”的最终工程形态应是：skill 负责理解任务和工作流，case_source_envelope 负责来源信封，Codex 自主选择 source-backed 路径；MCP 只提供可审计的来源、执行、验证和交付基础设施。这样才能覆盖未来用户任意追问、改口径、继续追一层和特殊案件问题。

### 0.0.3 二次污染防线：MCP/helper 与 Workbench 不得形成两套事实

真实风险不是“MCP 返回得全不全”这么简单，而是 MCP/helper 先返回一个固定口径，Codex 后续又查 DuckDB/workbench 得到另一个口径，最后两套结果在回答、报告或图表里混用，形成二次污染。Analytix 必须像 Data Analytics 一样把语义层当地图，把 live source read / notebook / SQL / artifact 当可复核来源，并用统一 evidence ledger 管住 claim。

强制规则：

- MCP/helper 默认返回 `source_map`、`query_guidance`、`compact_evidence`、`validation_state`、`provenance` 和 `known_risks`，不得默认返回最终中文研判结论；做不到这一点的 MCP 直接砍出 production 默认面。
- 若 MCP/helper 返回聚合结果，只能标为 `controlled_fact`、`ranking_only`、`candidate`、`needs_review` 或 `blocked`，并写清 scope、grain、dedup、time_window、source_hash、query_id。
- Codex 后续用 DuckDB/SQL/Python/notebook/workbench 复查时，结果必须进入同一个 Evidence Ledger；不能在回答中同时引用两个未对齐来源。
- 同一 claim 出现多个候选结果时，必须执行 source-of-truth selection：比较清洗表/analysis index、query scope、holder/account/counterparty grain、去重口径、重复/换卡风险、时间窗和成功/失败交易口径。
- 若无法解释差异，输出 Claim Gap / Reconciliation Needed，而不是取最大值、取最近一次工具结果或把两者相加。
- 面向模型的技能和 reference 应优先告诉 Codex：该看哪些清洗表/analysis index、常见口径坑、重复/换卡/同事实风险、如何验证和如何交付，而不是要求它相信某个固定 MCP 的返回。

因此，`主体A -> 对手方B` 这类金额问题不应被设计成“某个 MCP 一次返回最终金额”。正确路径是：focused skill 识别 Pair Amount -> source envelope 给表/索引/口径地图 -> Codex 选择低成本 helper 或 workbench 聚合 -> Evidence Ledger 合并候选 -> data-quality/claim-review 标注重复/换卡风险 -> 最终回答只引用已选定 source-of-truth 的金额和边界。

#### 三插件对齐后的限制清除条款

Data Analytics、Investment Banking、Public Equity Investing 的共同点不是“让插件更会禁止”，而是把来源、验证、支撑材料和读者交付分层。Analytix 必须照此清除自创限制：

- 不得把 `MCP 优先` 写成 `只能 MCP`；不得把 `semantic tool first` 写成 `SQL/Python/notebook/DuckDB/shell forbidden`。这些分析手段在当前案件、只读、清洗/analysis scope、限行、purpose、validation、artifact/evidence boundary 下都是合法的 source-backed 路径。
- 不得把 `funds_investigate`、rank、frontdoor、card、answer draft 或 graph contract 写成最终成稿 owner。它们只能是 source map、candidate、compact evidence、validator 或 artifact bridge。
- 不得把 stop condition、`answer_card_complete`、`max_additional_tools` 写成跨轮硬停；它们只表示当前事实包足以回答或当前预算应收口，不能阻止用户改口径、继续追一层、做否定性搜索或进入 Workbench。
- 不得把支撑层字段换成中文后直接给模型复述。大 JSON 变成大段文本仍是污染；模型可见层只能是最小事实摘录、证据表、短边界和 owner 可用线索。
- 不得因为语义 MCP 缺能力而拒绝当前明确任务。正确做法是换到当前案件可回放的 DuckDB/SQL/Python/notebook/workbench 路径；若该路径也缺 source-of-truth，才输出来源缺口。
- 不得用“证据边界”替代分析。边界必须服务于结论、异常特征、资金意义、下游追踪和补证动作。

保留的限制只限于成熟插件同样会保留的边界：无来源不强答、弱来源不冒充、未验证计算不进结论、support/audit 不外露、假交付不通过、跨案事实不复用、系统 Codex 不污染、法律/权属/实际控制不越证据定性。

### 0.0.4 金融专业插件交付层：Data Analytics 母版 + IB/PE 判断层

2026-06-11 真机复核显示：仅按 Data Analytics 补齐 source、validation、delivery 仍不足以让 Analytix 达到商业级。当前回答能给出若干金额和口径，但仍像 fact-card 摘要，缺少专业经侦判断：没有大额交易表、异常特征、资金意义、关系/角色线索、下游追踪和补证动作。Analytix 必须继续吸收本机 `Investment Banking` 与 `Public Equity Investing` 插件的专业金融交付纪律。

必须吸收的成熟机制：

| 成熟插件机制 | Analytix 经侦化转译 |
| --- | --- |
| Investment Banking router 只做 invocation gate、preflight 和 lead skill selection，不做实质业务。 | `index`、root skill、`funds_investigate` 只做入口、navigator 和能力发现；不得作为最终中文研判 owner。 |
| Investment Banking lead skill 拥有 deliverable intake、深度、artifact hierarchy、final response；support skills 只服务 source/model/QC/style。 | `quick-fact`、`subject-dossier`、`fund-tracing`、`full-case-analysis`、`visual-evidence`、`report-builder` 才是金额核验、主体画像、资金穿透、全案研判、图表/报告的 lead workflow owner。 |
| Public Equity Investing 要求 human-readable hero artifact first，support/audit files 留幕后。 | 用户先看到经侦研判结论、资金表格、流向图、报告或补证清单；SQL、ledger、workflow、case_id、debug label、handoff JSON 只留在 support/audit。 |
| Public Equity Investing 的 PM judgment heuristics 要求回答 so what、证据缺口和行动。 | Analytix 的 `investigative_judgment_layer` 必须回答资金案件意义、异常点、不能证明的边界、下一步侦查动作。 |

新的产品硬标准：

- `A 转给 B 多少钱` 是两方金额核验材料，不是单点查数。默认交付应包含结论句、统计期间、笔数和金额、主要集中日/集中账户、大额交易 Top 表、竞争金额的业务含义、重复/换卡/同事实风险、异常特征扫描、关系/角色或资金用途线索、下游追踪建议。用户明确只要一句数字时才压缩。
- 主体画像必须输出账户基本情况、账户清单、进出账概况、重点账户、重点对手方、异常特征、可疑用途和核查建议，不得用 `direct / candidate`、`Subject Dossier` 等内部分类替代业务表述。
- 资金流向图必须围绕资金来源、主链、下游去向、资金断点、待补证事项组织，不能把“口径与来源边界/数据事实/已有数据支持的资金边”作为读者结构。
- 报告/图表/附件必须像公安经侦材料：结论先行、事实表支撑、研判意见明确、证据边界短写、补证建议可执行。
- 质量门必须从“有没有内部词”升级到“有没有专业判断”。事实正确但没有研判意义、Top 表、异常扫描和下一步动作，仍判失败。

### 0.0.5 成熟插件技能结构标准与 Analytix 最终重构

深入复核 Data Analytics、Investment Banking、Public Equity Investing 后，Analytix 的最终结构不能再以 `funds_investigate` 的自然语言答案卡为中心。成熟插件共同点是：

1. **Router 极薄**：Data Analytics `index`、IB router、PE router 都只做 admission、preflight、focused skill selection；它们不继续执行业务，不写最终结论，不选择具体 artifact 细节。
2. **Focused skill 是 owner**：`build-report`、`company-tearsheet`、`memo-builder`、`deck-report-qc` 都有 Use When / Do Not Use / Workflow / Output Contract / Completion Gate；它们拥有最终用户交付责任。
3. **Support layer 隐身**：source-of-truth、normalizer、cleaner、QC、handoff JSON、manifest、logs、CSV 都支撑 owner，但不成为用户可见主答案。
4. **Hero deliverable first**：用户先看到报告、memo、tearsheet、QC verdict、workbook cover 或清晰 chat answer；support artifacts 只在需要时提一句。
5. **判断层不可省略**：Data Analytics 要 evidence + interpretation + implication；IB 要 banker decision hinge；PE 要 PM judgment；Analytix 必须对应经侦判断层。

因此 Analytix focused skills 最终分层如下：

| 层级 | Skill / 能力 | 最终定位 |
| --- | --- | --- |
| Router | `index`、root compatibility skill | 只做入口、当前案件状态、路由和能力发现；不得调用 `funds_investigate` 生成最终答案。 |
| Source/context support | `case-context`、casegraph/fundgraph、scope map | 提供案件来源、清洗/analysis 范围、图谱地图和 gap；不是结论。 |
| Bounded lookup | `quick-fact` | 只保留严格命中、排除、Top/ranking、单一低风险指标；不得继续承担金额争议、穿透或主体画像。 |
| 金额核验 owner | 新增 `pair-amount-investigation` 或等价 focused skill | 专门处理 `A 转给 B 多少钱`、金额挑战、重复/换卡/同事实、raw/effective/principal 口径，输出两方金额核验材料。 |
| Dossier owner | `account-dossier`、`subject-dossier`、`counterparty-analysis` | 账户/主体/对手方画像，含账户清单、资金进出、重点对手方、异常特征、补证建议。 |
| Trace owner | `fund-tracing`、`graph-visualization` | 资金来源、去向、下一跳、流向图和穿透图；只画可证实交易边，旁路线索分层。 |
| Typology / relation owner | `investigation-lab`，必要时新增 `relationship-network-analysis` | 团伙/共同控制/IP/MAC/电话/地址/单位/渠道/平台/税票合同/虚拟资产等专题线索，不升级为法律结论。 |
| Asset / product owner | 可由 `investigation-lab` 承担，若高频则新增 `asset-product-analysis` | 理财、证券、基金、购房购车、大额消费、现金、跨境、平台商户、虚拟资产去向专题。 |
| Reproducible analysis | `case-workbench` | 只读、当前案件、清洗/analysis scope 的 SQL/Python/notebook 专项核算；support，不是默认最终答案。 |
| Delivery owner | `visual-evidence`、`report-builder` | 表格、Top20、图谱、看板、附件、报告材料；必须有 hero deliverable 和 QA。 |
| Quality owner | `data-quality`、`claim-review`、`analysis-critique`，必要时新增 `delivery-qc` | 复核来源、口径、重复、图谱边、法律敏感表述、专业判断层和用户可见质量。 |

必须大删/大改的旧结构：

- `funds_investigate` 不能再输出可直接复述的最终中文研判。它只能返回 `source_map`、`compact_evidence`、`query_guidance`、`validation_state`、`known_risks`、`next_owner_skill` 和 support facts；若保留 `answer_draft`，只能作为 support note，不得成为默认 agent-readable 主体。
- `quick-fact` 中“先 call funds_investigate，使用 answer draft 后 stop”的合同必须删除或降级；金额核验从 quick-fact 拆出独立 owner。
- root skill 中“首个内部动作应调用 funds_investigate”的合同必须删除；改为像 Data Analytics/IB/PE 一样先 route 到 focused skill owner。
- `card-renderer` / `agent-output-compiler` 不得拼出用户最终答案样式标题和段落；它们只能提供证据包、事实要点、风险和下一步 owner，最终中文交付由 focused skill / Codex 生成并由 critique gate 检查。
- tool schema 不得写“普通桌面问法应优先使用 funds_investigate”；应写“可作自然语言 navigator，明确 lane 直接选 focused semantic tool”。
- capability registry、command metadata、doctor、eval 必须以 focused owner 为上游事实源，而不是以 `funds_investigate` 前门为中心。

完成判据：

- 一个用户问题最终只能有一个 lead focused owner；support tools 可以多个，但用户正文不得暴露 support 字段。
- 金额核验、主体画像、资金穿透、全案报告四类真实任务都能在最终答案中体现经侦判断层：事实表、异常特征、资金意义、证据缺口、下一步。
- 若最终答案只复述 MCP/card 的标题、金额和口径，即使金额正确，也视为商业级失败。

### 0.0.6 经侦专家交付层：从“查数”升级为“可办案研判材料”

继续复核 Data Analytics、Investment Banking、Public Equity Investing 后，Analytix 还必须补齐一个比 source/validation/delivery 更上层的产品能力：**经侦专家交付层**。成熟插件不是只保证数据正确，它们还保证专业读者拿到的是可决策、可复核、可行动的材料：

- Data Analytics `build-report` 要先形成 report spine：直接答案、关键证据、解释、caveat、next step；报告不是图表/表格堆叠。
- Investment Banking `memo-builder` 要先定义 memo plan、decision hinge、load-bearing claims、source posture、open items、recommended next step；support 文件不进入 hero deliverable。
- Public Equity Investing `memo-builder` 要先形成 recommendation / decision ask、what must be true、disconfirmers、monitoring、source posture；事实必须转成 PM judgment。

Analytix 的经侦化转译如下：

| 成熟插件能力 | Analytix 经侦化能力 |
| --- | --- |
| Report spine | `案件研判 spine`：问题/对象、直接结论、控制口径、关键事实、异常特征、资金意义、证据缺口、下一步侦查动作。 |
| Memo plan | `研判材料 plan`：材料用途、读者、案件范围、来源成熟度、核心金额/账户/链路、待核事项、可报送状态。 |
| Evidence posture | `证据成熟度`：已证事实、强支持事实、线索、待核假设、暂不能认定、阻断缺口。 |
| Decision hinge / PM judgment | `侦查判断点`：这笔钱/这组账户为什么重要、异常在哪里、与主体关系如何、还缺什么材料才能认定、下一步查谁/查什么/调什么。 |
| Hero deliverable first | `办案材料优先`：两方金额核验材料、主体账户研判、资金穿透图、专题研判报告、补证清单优先；support ledger、SQL、workflow 隐身。 |

#### 经侦输出成熟度标签

普通用户可见输出不得使用 `screen-grade`、`support layer`、`delivery_state` 等金融/工程词。内部可以保留机器字段，但用户正文必须使用下列经侦业务表达：

| 内部状态 | 用户可见表达 | 适用条件 |
| --- | --- | --- |
| supported / verified | 已有流水支持 | 金额、笔数、账户、交易边可由当前清洗/analysis 事实支撑。 |
| high-confidence | 高可信支持 | 同事实/换卡/重复风险已做针对性核验，仍支持该结论。 |
| candidate / lead | 线索 | 共同设备、同电话、同地址、同对手、摘要关键词、现金桥等只提示侦查方向。 |
| needs_review | 需复核 | 存在重复、换卡、对手缺失、账户归属、时间窗、join 或来源冲突。 |
| blocked | 暂不能认定 | 缺少关键流水、回单、账户维表、支付平台资料、资产登记或其他控制来源。 |
| final-circulation-candidate | 可形成材料候选 | 事实、口径、附件、图表、claim review 已达到报告候选标准。 |

#### 核心交付形态的读者结构

1. **两方金额核验材料**：以“经梳理，某期间 A 向 B 转账 X 笔，合计 X 元”开头；随后给大额交易表、账户/时间集中、raw/effective/duplicate 统计范围差异的业务解释、异常特征、关系/资金目的线索、下游追踪建议。不得先写当前案件预检、同名聚合支持或核心账号范围等支撑层标题。
2. **主体/账户研判**：先讲基本情况和账户清单，再讲进出账规模、重点账户、Top 对手方、账户角色、资金来源/去向、生活卡/中转/汇聚/资产消费/理财证券/现金/境外等特征，最后给核查建议。
3. **资金流向图**：按资金来源、主链、下游去向、资金断点、旁路线索、补证事项组织；图前给一句结论，图后解释链路意义。不得把审计边界、数据事实或图谱依据表作为标题。
4. **全案研判**：先给案件数据盘点和总体判断，再按主体、账户、资金规模、链路、异常专题、关系网络、资产/产品/现金/境外、补证清单组织；不是全工具日志，也不是报告模板填空。
5. **正式报告/附件**：仿照 IB/PE 的 hero artifact，报告和附件必须是读者主交付；聊天只做交付说明。报告 claim、表格、图谱、金额单位和证据边界必须复核。

#### 需要新增或强化的 skill

- `pair-amount-investigation`：必须新增或等价落地，处理金额核验和金额争议。
- `delivery-qc` 或扩展 `analysis-critique`：必须覆盖专业判断层、经侦口吻、表格/图谱/报告读者结构、support 隐身、材料成熟度，不只扫内部词。
- `relationship-network-analysis`：若团伙/共同控制/IP/MAC/电话/地址/共同对手/同设备线索高频出现，应从 `investigation-lab` 拆出 focused owner；否则至少作为 Lab 的一等专题。
- `asset-product-analysis`：若购房购车、理财证券基金、保险、支付平台、虚拟资产、境外资金高频出现，应从 Lab 拆出 focused owner；否则至少作为 Lab 的一等专题。
- `report-builder` 必须吸收 memo-builder 做法：材料用途、读者、证据成熟度、核心 claim、附件状态、待核事项、可报送状态先行，不能只套公安报告模板。

任何最终答案只要出现以下现象，即使数字正确也判失败：只解释口径、不解释侦查意义；只有金额无表格；有图无链路判断；有主体统计无账户清单；有“需复核”但不说复核什么；有补证建议但不列对象、字段、时间窗、证明目的；外露 workflow、case_id、support 字段、英文标签或工程标题。

### 0.0.7 Analytix 软件工作流智能层：让 Codex 更懂 Analytix，而不是背模板

复盘两个无插件旧线程后，Analytix 插件的真实目标必须重新表述为：

> 让 Codex 更懂 Analytix 软件里的案件、清洗表、分析索引、资金图谱、报告和附件工作流；让 Codex 在 Analytix 数据和工具加持下，更懂资金数据分析，并能产出公安经侦专家级的深度研判和高质量材料。

这两个无插件线程暴露出的成功模式不是“提示词长”，而是 Codex 会根据用户推进不断切换工作层：

| 无插件线程成功行为 | Analytix 插件必须产品化 |
| --- | --- |
| 用户要求完整报告时，先理解 Analytix 数据分析功能树、案件数据量、清洗覆盖、主体/账户/对手方和报告框架。 | `full-case-analysis` 必须懂 Analytix 左侧功能树和案件工作流，自动跑数据盘点、主体/账户盘点、Top、异常专题、链路、续调清单，而不是只给计划或摘要。 |
| 用户追问某人/某卡时，会回到清洗交易明细和账户维表，核对字段、别名、时间窗、方向、现金标识、摘要、重复和空对手。 | `pair-amount-investigation`、`subject-dossier`、`account-dossier` 必须优先理解清洗表语义和 Analytix 导出口径，必要时生成与 Analytix 导出一致的明细/附件。 |
| 用户要求“继续追一层/画图/导出图片”时，会把已核实资金边转成可读 Mermaid/PNG/JPG，并把未调取端点放入补证方向。 | `fund-tracing`、`graph-visualization`、`visual-evidence` 必须懂图谱展示和报告插图工作流；有可用 Analytix/Codex 交付面时生成并检查，暂无交付面时给证据表和交付缺口。图不是工具返回物，而是研判材料的一部分。 |
| 用户要求深挖项目款、主材劳务、关联公司、亲属和资产端时，会按侦查假设跑二级/三级链路，区分已闭合链条和扩展线索。 | `investigation-lab` 必须以侦查假设驱动，而不是以工具菜单驱动；能把成本外观、公司绕行、亲属/关联人、资产端、现金/理财/证券等专题分层。 |
| 用户要求“金额必须真实正确”时，会逐项重算报告金额、明细、图块、术语，发现展示口径容易误读就改写。 | `claim-review` 与 `delivery-qc` 必须复核事实、口径、图谱、附件和语言；不能只扫内部词。 |

#### 禁止模板化输出

Analytix 的 skill 不能把用户问题套进固定段落。成熟插件的做法是“稳定 workflow + 自适应读者结构”，不是“固定正文模板”：

- Data Analytics 的 report spine 会随问题、读者、证据和交付面变化。
- Investment Banking 的 memo plan 会随交易阶段、读者、source posture 和 decision hinge 变化。
- Public Equity Investing 的 memo spine 会随投资动作、证据强度、disconfirmers 和监控点变化。

Analytix 对应规则：

- 稳定的是工作流：当前案件 -> source envelope -> focused owner -> live/source-backed verification -> investigative spine -> delivery-qc -> material/artifact。
- 自适应的是正文：金额核验、取现明细、主体画像、资金穿透、项目款链路、资产端、报告、附件，各自按用户任务和已验证证据组织。
- 不能把“经梳理/重点交易明细/异常特征/补证建议”写成每次必然四段；只有证据支持、任务需要、读者有用时才展开。
- 简短问题不等于简陋回答；复杂任务也不等于堆满所有栏目。插件必须判断用户是在要数字、明细、研判、图、附件、报告，还是继续推进上一轮材料。

#### Analytix 原生工作流知识必须进入插件

插件必须让 Codex 理解以下 Analytix 原生工作流，而不是让用户反复解释：

1. 案件中心与当前案件：桌面端上次打开案件、当前选中案件、current case-project binding、analytix runtime _analytix.workspaceRealPath context、MCP case resolution 必须一致。
2. 数据导入与清洗：普通研判默认使用 Analytix 清洗表和 approved analysis 索引；原始导入表、fixture、旧输出不能冒充当前事实。
3. 清洗表导出口径：用户要“清洗明细表/导出明细/原始清洗字段”时，走显式受审批的 `export_cleaned_case_data`，保持 Analytix 导出中文字段、sheet、字段顺序和可回查性；任务未完成、文件未生成或读取检查未通过时不得宣称已交付。
4. 分析索引与图谱：analysis 表、casegraph、fundgraph 是找数和组织证据的地图；不是最终结论。
5. 报告与附件：报告不是聊天摘要；附件、XLSX、PNG/JPG、Mermaid、表格和证据包都要有 owner、质量检查和可回放来源；插件只协调和验收可用交付面，不自行实现通用 PDF 渲染、OCR、资产登记或外部证据获取系统。
6. 多轮状态：继续、改口径、补图、导出、回到清洗表、核对全文金额等都属于本案工作流延续；不能把每轮当孤立问题。

完成标准：真实用户在 Analytix 桌面端问“某人资金研判/某人转给某人多少钱/画流向图/继续追一层/导出明细/生成报告”时，Codex 应自然体现“我懂 Analytix 这个软件怎么做这件事”，而不是外露插件内部流程、MCP 工具说明或固定模板。

## 0. 总纲与产品硬标准

### 0.1 核心判断

```text
Analytix 涉案资金研判插件是安装在 Analytix 内嵌 Agent 中的专业能力插件包。
它通过 Skill + MCP + references + scripts + assets，为 Codex 提供证据约束下的涉案资金研判能力层。
插件本身不是独立业务系统，不替代 Analytix 后端、前端、案件管理、数据平台或人工法务判断。
插件不能退化成普通事实查询入口；它必须在插件边界内组织侦查意图、开放假设、确定性事实、证据复核、多轮追查和报告交付约束。
```

### 0.2 总目标

Analytix 涉案资金研判插件的总目标是：

```text
把通用 Codex 与人工多轮研判已经证明有效的自由探索能力，
产品化为 Analytix 内部可复核、可审计、可交付、可持续评估的涉案资金研判能力。
```

这里的“产品化”指插件包能力产品化，不是另起一套独立资金研判业务系统。插件的身份始终是 Analytix 内嵌 Agent 的专业能力增强层；案件事实、权限、数据清洗、索引、审计、导出、UI 展示和最终人工判断仍由 Analytix 主系统与人工流程承载。

### 0.3 总目标分层

| 目标层 | 定义 |
| --- | --- |
| 产品目标 | 把自由探索、数据复核、链路追查、证据复核和报告交付整合成 Analytix 内嵌 Agent 的专业插件能力。 |
| 业务目标 | 支撑涉案资金事实梳理、主体/账户画像、资金链路穿透、专题线索研判、补调清单和报告材料生成。 |
| 工程目标 | 形成 Skill + MCP + Capability Registry + Command Router + deterministic runtime + Evidence Ledger + Context Compiler + Claim Verifier 的稳定链路。 |
| 质量目标 | 在事实准确、追查深度、幻觉抑制、报告交付、成本控制和隔离边界上可评估、可回放、可核准。 |
| 风险目标 | 防止无来源强答、弱来源冒充、未验证计算、假交付、长 prompt、大 JSON、工具盲扫、unsupported Mermaid、候选归属升级和跨案污染。 |
| 商业目标 | 形成顶级商业成熟插件：真实案件压力下仍能稳定输出经侦风格、证据可复核、成本可控的资金研判材料。 |

### 0.4 核心目标沉淀

| 目标 | 在插件中的产品化定义 |
| --- | --- |
| 侦查意图驱动 | Codex 必须能先理解用户真实问题，再自主选择语义事实工具；Task Profile / Intent AST 只辅助复杂任务和评测，不得替代模型判断。 |
| 开放式假设 | Codex 可以自由提出金额集中、时间集中、换卡、现金断点、理财、资产、支付通道、缺失对手方等假设；Investigation Lab 用于标注状态和证据缺口，不用于垄断开放深挖。 |
| 数据复核 | 金额、笔数、账户、户名、对手方、方向、时间窗口、去重口径只能来自 Deterministic Tool Runtime 和 Evidence Ledger。 |
| 多轮追查 | 每轮都允许 Codex 基于新问题、新对象、新时间窗或新口径继续选择工具；casegraph / fundgraph 负责承接多轮上下文和证据回溯。 |
| 质级提升 | 在资金分析、资金穿透、线索研判、证据复核、报告生成上，必须用 doctor、runtime health、功能闭环、报告压测和人工抽检证明 Data Analytics 式 source + validation + delivery 治理真实落地。 |
| 禁止退化 | 不能退化成普通事实查询入口、说明书式插件、长 prompt、大 JSON、低层工具盲扫，或由单一前门替 Codex 思考。 |

### 0.5 五项产品硬能力

| 维度 | 必须形成的质级提升 |
| --- | --- |
| 事实更准 | 金额、笔数、账户、户名、对手方、方向、时间窗口、去重口径来自确定性事实层，不能由模型猜测。 |
| 追得更深 | 能沿资金来源、去向、金额集中、时间集中、换卡、现金断点、理财、资产端、缺失对手方继续提出追查链。 |
| 更少幻觉 | 假设、线索、需复核、证据缺口、禁止写成事实的内容必须分层，不能把推测写成已查明。 |
| 更可交付 | 输出像经侦资金研判材料：先事实，后口径，再边界，最后补证建议和报告门禁。 |
| 更高认知效率 | 默认暴露高价值语义事实工具，让 Codex 少扫低层 raw rows 但能自主选择路径；raw payload 留在本地 artifact / MCP `_meta`，模型看到的是可行动事实而不是内部协议。 |

### 0.6 产品判据

| 判据 | 成立条件 |
| --- | --- |
| 能理解侦查意图 | 用户不写命令也能进入正确 lane；命令只是稳定入口，不是工具菜单。 |
| 能自由提出假设 | Codex 可以提出异常金额集中、时间集中、换卡、现金、理财、资产、支付通道、缺失对手方等假设。 |
| 能确定性核事实 | 所有金额、笔数、账户、户名、对手方、交易边、附件行数必须来自事实层和 Evidence Ledger。 |
| 能多轮追查 | 每轮都有下一步、证据缺口、收口边界、禁止事实化内容和可复用的本案上下文。 |
| 能报告交付 | 报告、附件、Mermaid、续调清单和重大 claim 都能被 Claim Verifier、Critique Pass 或专项校验阻断/纠正。 |
| 能高效自主运行 | 普通问答可直接选用语义工具或自然语言快捷入口；复杂任务按 Plan DAG 升级预算；raw payload 不进正文，工具选择权不被前门垄断。 |
| 能保持隔离 | 插件状态、runtime、artifact、trace、ledger、eval 不污染系统 Codex，不跨案复用事实。 |

### 0.7 非目标

插件明确不做：

- 不把插件做成独立业务系统。
- 不让插件接管 Analytix 后端、前端、案件数据平台、导出链路或人工审核。
- 不把 Case Workbench 做成无来源、无验证、无交付状态的查询结果搬运器；受控 SQL/notebook 是可复核分析能力，不是报告级事实豁免。
- 不提供法律、税务、资产归属或最终责任认定引擎。
- 不把第三方遥测、跨案长期记忆、全局 Codex hooks 引入 Analytix。
- 不用大模型主观判断替代确定性事实层。
- 不用报告模板堆砌替代证据复核和资金穿透。

### 0.8 不可退化标准

插件不能降级为：

- 多写几个工具。
- 多写一段 prompt。
- 多塞一份 JSON。
- 多加一个长 skill。
- 让模型把未验证计算、弱来源、样本行或局部预览自行升级为事实和 claim。
- 把 Controlled Case Workbench 退化成无来源信封、无验证状态、无 evidence boundary 的查询结果搬运器。
- 用静态存在性检查替代真实质量证据。
- 用报告末端校验替代全链路输出准则。
- 用图谱展示替代 supported transaction edge。
- 用命令菜单替代受控意图路由。

核心硬标准：

| 硬标准 | 具体含义 |
| --- | --- |
| 证据门禁 | 每个报告 claim、资金边、Mermaid 箭头、法律敏感表述都要能被 Evidence Ledger、Claim Verifier 和 Critique Pass 校验。 |
| 专业交付 | 最终材料像公安经侦资金研判材料，先给事实、再给口径、再给边界、再给追查建议。 |
| 低上下文成本 | raw output 不进模型上下文，模型只看 Context Compiler 生成的研判事实卡。 |
| 低工具成本 | 普通事实题一次资金研判调用即可回答，报告题最多一次事实卡加一次报告复核。 |
| 案件隔离 | facts、ledger、trace、eval artifact、hypotheses 都在 Analytix-owned 案件范围内闭合。 |
| 可持续核准 | 每轮推进必须能用 doctor、guard、runtime health、功能闭环、报告压测证明能力没有退化。 |
| 自主研判保持 | 插件限制不受控取数和无证据结论，但不能限制 Codex 的提问、假设、反证、追查方向选择和报告组织能力。 |

### 0.9 成熟插件形态定稿：Data Analytics 母版 + Impeccable 质控

Analytix 的终极形态不是在 Impeccable 和 Data Analytics 之间二选一，而是：

```text
产品形态学 Data Analytics：多 skill 任务入口 + index 路由 + 轻量状态/上下文预检 + focused workflow + source-backed verification + delivery sub-skill。
判断质量学 Impeccable：领域词汇 + 反模式规则 + deterministic detector + LLM critique pass。
```

这一定稿进入蓝本最高产品约束：

- Data Analytics 是产品母版，不是并列参考项。Analytix 的 route、skill、preflight、workbench、validation、delivery 和性能策略，均先按 Data Analytics 成熟插件做法建模，再叠加经侦资金领域方法。
- `Analytix` 不能长期停留在一个大而全的 `analytix-fund-analysis` skill。单 skill 可以作为 MVP 或兼容入口，但顶级商业成熟插件必须有多 skill 产品面，让用户和模型都能看见“某卡研判、某人研判、全案分析、资金穿透、数据质量、报告生成、claim 复核”等任务入口。
- 多 skill 不是把 MCP tool 一对一包装成 skill。`rank_accounts`、`trace_fund`、`analyze_holder_full` 等仍是共享语义事实工具；skill 负责触发语义、工作流边界、完成标准和渐进披露。
- `index` skill 只能像 Data Analytics 的 `index` 一样做路由和能力介绍，不能重新变成 `funds_investigate` 式总指挥。
- `case-context` / `case-scope` skill 应像 Data Analytics 的 `user-context` 一样做当前案件、scope、语义索引、运行状态和上下文预检，但只能保存 Analytix-owned、本案内状态，不能成为跨案长期记忆。
- `visual-evidence` 应学习 Data Analytics 的 `visualize-data`、`build-dashboard`、`spreadsheets` 和 `build-report` 交付纪律：先定义证据问题和读者动作，再选择表格/图表/看板/附件；每个视觉输出都必须有 scope、unit、time window、metric、direction、evidence status 和 QA，不得用图表制造新案件事实。
- Impeccable 式命令、反模式和 critique 必须覆盖普通问答、排行、对象画像、图谱、Lab、全案分析和报告，而不能只在报告末端兜底。

Data Analytics 的可借鉴面按“产品形态和交付纪律”吸收，不搬商业 KPI 语义，也不依赖 Data Analytics runtime：

| Data Analytics skill | Analytix 应吸收 | Analytix 不吸收 |
| --- | --- | --- |
| `index` | 轻量路由、能力介绍、focused skill 选择。 | 在 index 中执行全案研判或替 Codex 决策下一步。 |
| `user-context` / `gather-business-context` | `case-context`：当前案件、清洗表覆盖、开户/联系/住址/环境字段、casegraph/fundgraph 预检。 | 跨案长期记忆、公司业务语义层、全局偏好写入。 |
| `analyze-data-quality` / `validate-data` | `data-quality`：清洗覆盖、重复/换卡、空户名、缺失对手、口径一致性、报告前质量门。 | 让模型读原始表或把样本行当全量统计。 |
| `product-business-analysis` / `metric-diagnostics` | `full-case-analysis`、`investigation-lab`、`analysis-critique`：围绕决策问题做分解、异常解释、反证和下一步侦查建议。 | 把经侦案件写成商业增长、KPI 或市场机会分析。 |
| `visualize-data` | `visual-evidence`：Top20、集中度、时间分布、特征矩阵、流向表的图表选择和 QA。 | 用图表推导未复核事实、资金路径或法律结论。 |
| `build-dashboard` | `visual-evidence`：案件看板卡、范围覆盖、重点对象、可疑特征、链路断点、补调进度。 | 把看板变成实时 BI 产品或替代正式研判报告。 |
| `spreadsheets` | `visual-evidence` / `evidence-request`：附件清单、续调清单、Top 对手表、证据工作簿。 | 裸导全量流水、绕过 Analytix 权限和审计。 |
| `build-report` | `report-builder`：读者优先、证据相邻、表图进入报告前先 QA、材料可审计。 | chat summary 冒充报告；报告生成先于事实树和 claim review。 |
| `jupyter-notebooks` | `case-workbench`：当语义工具不足以覆盖明确自定义口径时，提供当前案件、只读、清洗表/分析索引、限行、可回放的受控 SQL/notebook 分析。 | 面向普通办案用户把 notebook 作为默认路径、把未验证计算当结论、或引入跨案语义层。 |
| `design-kpis` / `kpi-reporting` / `market-sizing` | 只吸收“定义口径、目标、边界、对比基准”的方法。 | KPI 管理、市场规模测算、商业运营语义。 |

成熟产品面最低包含三层：

| 层 | 定义 | Analytix 落点 |
| --- | --- | --- |
| 用户可见 skill | 用户和模型能直接理解的业务任务入口。 | `index`、`case-context`、`quick-fact`、`account-dossier`、`subject-dossier`、`fund-tracing`、`full-case-analysis`、`report-builder`、`visual-evidence`、`case-workbench` 等。 |
| 共享 source-backed 能力层 | 被各 skill 复用的来源、查询、验证和交付能力；MCP 只是承载形态之一。 | scope、rank、profile、trace、casegraph、hypothesis、quality、validation、navigator、controlled workbench、artifact/provenance。 |
| 质控与交付子流程 | 不一定顶到用户技能列表，但支撑交付质量。 | claim review、continuation list validation、graph QA、report export、runtime doctor、functional eval。 |

验收时不能只看 skill 数量。合格标准是：用户说“某卡资金研判”会进入对象画像 skill，用户说“全案分析”会进入全案分析 skill，用户说“生成报告”才进入报告 skill；每个 focused skill 都有自己的触发语义、非目标、最小工作流、证据门禁、完成标准和 eval prompt。

### 0.10 Data Analytics 对齐后的自由探索合同

Data Analytics 的成熟做法是保留 SQL/notebook/Python 作为可复核分析路径，而不是靠硬停止替模型决定下一步。它的核心是：先用语义层和权威源定位口径，再做 live source verification；当常规语义层不足时，用 notebook/SQL/Python 形成可复核计算路径；最后通过可视化、报告、dashboard 或 notebook 的交付门禁，把计算结果变成可读、可审计、可复用的分析材料。

Analytix 必须建立 Data Analytics 等价规则：

```text
Data Analytics 没禁止的工具，Analytix 不得自创工具禁令。
Data Analytics 禁止的，是无来源强答、弱来源冒充、未验证计算、假交付、隐藏状态误读和内部实现外泄；Analytix 只做这些机制的经侦等价约束。
```

Analytix 对齐后的合同是：

- `case-context` 对应 Data Analytics 的 context/semantic-layer preflight：先确认当前案件、清洗表/分析索引覆盖、开户/人员/联系/住址/环境字段、casegraph/fundgraph 和 source gap。它只保存 Analytix-owned、本案内状态，不保存跨案长期记忆。
- 语义 source helpers 对应 Data Analytics 的 source-backed analysis：当现有语义 helper 能覆盖任务时，它是低成本候选路径；当它只给地图、排行线索、样本、弱口径或无法覆盖明确计算时，Codex 必须能切换到其他来源、quality/critique 或 Controlled Case Workbench。
- `case-workbench` 对应 Data Analytics 的 notebook/SQL fallback：当用户明确提出 SQL、notebook、自定义口径、可回放计算，或 focused skill 能说明现有语义工具缺口时，允许在当前案件、只读、清洗 `fc_*_norm` / `analysis_*`、限行、purpose、审计 artifact 边界内执行受控自定义分析。
- `visual-evidence` 和 `report-builder` 对应 Data Analytics 的 delivery skills：表格、Top20、图表、看板、附件、正式报告都必须有读者问题、证据来源、口径、视觉/表格 QA 和交付状态，不能用聊天摘要替代交付物。
- `data-quality` 和 `claim-review` 对应 Data Analytics 的 validation quality bar：缺失源、schema 暂不可用、清洗/索引覆盖不足、候选归属、bounded preview、unsupported edge 都必须转成风险、降级或阻断。
- `delivery-qc`、`report-builder` 和 `claim-review` 进一步吸收 `567-labs/instructor` 的 schema-first 思想：final answer 和报告段落可以是自然语言，但生成前必须落入 `FinalInvestigationAnswer` 的结论、已核验事实、支持资金路径、异常特征、证据边界和补证动作骨架，并通过 `validate_report_claims(final_investigation_answer)` 接入确定性复核；结构缺失可修复，事实缺失只能回 MCP/DuckDB 或降级为边界。

Data Analytics 的十五项核心原则在 Analytix 中不可缩水：

| 原则 | Analytix 经侦等价实现 |
| --- | --- |
| preflight envelope | 先形成 `case_source_envelope`，确认任务上下文、当前案件、可用清洗表/analysis index、casegraph/fundgraph、目标任务和交付义务。 |
| semantic layer as map | casegraph、fundgraph、scope map、schema 和 command metadata 只做找数地图，不直接构成结论。 |
| source-of-truth selection | 金额、笔数、账户、主体、对手方、路径、图表和报告 claim 必须选择控制来源，并说明冲突来源优先级。 |
| source guardrail | 必需来源缺失时阻断对应报告级 claim；可选来源缺失时继续但标注 gap、影响和补调方向。 |
| live/source-backed verification | 数据结论必须实际通过 source helper、受控 SQL/notebook、backend 计算或 evidence ledger 复核，不能凭样例、记忆或工具说明作答；“工具返回了行”不等于该行可作为最终 claim。 |
| SQL / Python / notebook allowed when useful | Data Analytics 没禁的分析手段 Analytix 不自创禁令；语义 helper 是低成本路径之一，不是唯一事实路径；语义工具不足且任务明确时进入受控 workbench。 |
| data quality checks | 对粒度、时间窗、缺失、重复、join 风险、异常值、freshness、清洗覆盖和口径漂移做高信号检查。 |
| validation state | 明确已验证、部分验证、线索/可能、待复核、阻断和不可用，不把线索写成已查明事实。 |
| delivery contract | 报告、图表、表格、notebook、证据包和附件包只有在可用交付面生成并检查后才可宣称交付；否则必须明确 blocker。 |
| render/execute QA | 报告要打开/渲染检查，notebook/workbench 要记录执行状态，图表/表格要在最终容器检查。 |
| audience language | 用户只看研判结论、口径边界和下一步；内部状态、评测器、诊断标签、raw rows、大 JSON 和插件实现留在 audit/evidence。 |
| index / navigator not commander | root/index/funds_investigate 只做能力发现、意图归类和 focused skill 选择，不执行全案研判或替 Codex 决定下一步。 |
| focused workflow ownership | 每个 focused skill 必须有 Use When、非目标、workflow、completion gate 和质量门；root skill 不能替代对象画像、全案、追踪、报告、图表或 workbench。 |
| report completion gate | 一旦进入报告路线，不能用 inline/chat summary 结束；必须完成报告形态、claim 复核、证据/图表/附件交付或明确 blocker。 |
| schema-first final answer | 借鉴 Instructor 的 response model / validation / reask：最终回答背后要有 `FinalInvestigationAnswer` 结构，缺证据事实触发 validation failure；报告级文本必须把该结构作为 `final_investigation_answer` 进入 `validate_report_claims`；只允许修结构，不允许模型补事实。 |
| scoped state / memory | 状态只保存 Analytix-owned、本案内 source、scope、artifact、journal 和 semantic-map 指针；不保存跨案事实记忆、全局偏好或系统 Codex 状态。 |

能力缺口处理不能再次变成机械限制：

```text
先换 source-backed 路径 -> 仍不足且任务明确 -> 进入 Controlled Case Workbench -> 返回当前任务的受控结果或 Gap Card -> 把可复用缺口登记为 source helper / MCP 产品化候选
```

这意味着“沉淀为新的 MCP 工具”是产品改进闭环，不是当前案件回答的前置条件。当前任务能在受控 workbench 内回答时应先回答；只有 workbench 工具缺失、权限不足、清洗表缺口或安全门禁阻断时，才输出能力缺口。插件不得自我升级、自动改 backend、自动发布新 MCP；新增语义工具只能通过正常工程、测试、版本和 Hub 发布链路进入生产。

## 1. 插件身份与商业成熟定义

### 1.1 插件身份

`Analytix 涉案资金研判` 插件是 Analytix 内嵌 Agent 的能力增强层。它围绕选定案件工作，不接管主系统，不污染系统 Codex，不形成跨案件事实记忆。

插件由以下部分组成：

| 组成 | 角色 |
| --- | --- |
| Skill | 触发插件、声明边界、引导渐进披露。 |
| MCP server | 连接 analytix data/runtime backend，提供确定性事实工具、校验工具和本地 artifact。 |
| references | 承载命令矩阵、领域口径、运行边界、报告 schema、反模式和能力规范。 |
| scripts | 承载 doctor、eval、health、guard 和契约检查。 |
| assets | 承载插件分发、展示和必要静态资源。 |
| Capability Registry | 作为命令、能力、工具预算、门禁、eval、doctor 的机器事实源。 |

插件能力来自“Codex 侦查思考 + Analytix 确定性事实层”的组合。Codex 负责理解用户意图、提出假设、质疑口径、选择追查方向和组织材料；插件负责让金额、笔数、账户、户名、对手方、交易边、图谱边和报告 claim 都经过事实层、证据层和门禁层。

### 1.2 顶级商业成熟插件定义

顶级商业成熟插件不等于工具多、提示词长、报告模板多或流程复杂。商业成熟指插件在真实案件、真实用户、真实多轮追问和真实交付压力下，仍能稳定保持事实准确、研判深入、证据可复核、输出可交付、运行低成本、边界可治理。

| 成熟维度 | 系统定义 |
| --- | --- |
| 业务专业性 | 围绕涉案资金事实、侦查研判工作流、证据复核门禁和经侦材料交付组织能力，不扩成通用 agent 框架。 |
| 事实确定性 | 金额、笔数、账户、户名、对手方、方向、时间窗、去重口径和交易边来自 analytix data/runtime backend / MCP / deterministic runtime。 |
| 自主研判 | Codex 保留提问、假设、反证、口径质疑、追查方向选择和报告组织能力。 |
| 开放深挖 | 支持模糊、复杂、专题式、多轮式资金问题，能够从事实卡升级到假设队列、受控查询、图谱追查和报告复核。 |
| 证据治理 | 每个可交付事实、链路、图谱、附件和报告 claim 都有 Evidence Ledger、Context Compiler、Claim Verifier、Critique Pass 或人工确认链路。 |
| 专业交付 | 输出不是工具摘要，而是事实卡、研判卡、资金链路卡、补调清单、附件表、复核卡和正式报告材料。 |
| 成本治理 | 普通题低工具、低上下文、低重复调用；复杂题按 Plan DAG 计划化升级，并保留预算和收口边界。 |
| 隔离安全 | 插件状态、artifact、trace、ledger、eval、cache 保持 Analytix-owned，不污染系统 Codex，不跨案复用事实。 |
| 可核准性 | doctor、eval、health、runtime check、trace replay、报告抽检和人工复核形成质量证据。 |

### 1.3 插件包边界

| 边界 | 定义 |
| --- | --- |
| 不替代主系统 | 案件管理、权限、数据清洗、索引、导出、审计和 UI 展示仍由 Analytix 主系统承载。 |
| 不污染系统 Codex | 插件运行时、缓存、日志、artifact、证据账本和 eval 输出只属于 Analytix。 |
| 不跨案记忆 | 多轮追查状态只能在本案内复用，不能形成跨案件长期事实记忆。 |
| 不越权定性 | 法律、税务、资产归属、实际控制、最终责任等判断必须由外部证据和人工确认闭合。 |

资产展示边界：

- `assets/icon.png` 和 `assets/logo.png` 只承载 Hub、插件页和安装界面的视觉身份。
- 资产文件不承载业务规则、prompt、案件事实、工具配置、运行变量、证据账本或评测答案。
- 资产变更只影响展示一致性，不得被当作能力、质量、事实或发布成功证据。

### 1.4 Manifest、MCP Manifest 与界面元数据契约

插件 manifest 和 MCP manifest 是交付入口，不是产品能力本身。它们必须稳定表达插件身份、界面展示、技能挂载、MCP server、环境变量、工具审批和资产路径，不能承载案件事实、调试材料、评测答案或系统 Codex 配置。

`.codex-plugin/plugin.json` 字段契约：

| 字段 | 施工契约 | 禁止 |
| --- | --- | --- |
| `name` | 插件包稳定标识，与 registry、Hub package、runtime cache 和 skill mount 对齐。 | 随营销文案随意改名。 |
| `description` | 简明说明插件是 Analytix 当前案件资金研判能力。 | 写入过程记录、调试状态、评测结论。 |
| `author` / `homepage` / `repository` | 指向 Analytix 官方身份或受控发布源。 | 指向个人路径、本地路径或非受控仓库。 |
| `license` | 商业插件可使用 `UNLICENSED` 或等效受控许可表达。 | 暗示开源再分发或外部授权。 |
| `keywords` | 只用于插件检索和归类。 | 作为能力路由或安全边界来源。 |
| `skills` | 指向插件内 skill 目录。 | 指向系统 Codex skill 或外部路径。 |
| `mcpServers` | 指向插件内 `.mcp.json`。 | 内联系统 MCP 配置或全局工具配置。 |
| `interface.displayName` | 用户可见品牌名。 | 写过程、调试、评测或内部 intent 名。 |
| `interface.shortDescription` | 插件页短说明，聚焦当前案件资金穿透、账户画像、报告。 | 写夸大承诺、法律结论或最终归属能力。 |
| `interface.longDescription` | 可列能力全集，但必须是产品化能力描述。 | 写内部测试、历史过程、真实案件样例或 eval 答案。 |
| `interface.capabilities` | 只表达插件交互和读写类型。 | 将 `Write` 解释为可写系统配置、数据库或全局 Codex 状态。 |
| `interface.defaultPrompt` | 提供少量稳定入口示例。 | 承载核心 prompt、过程性表述或长工作流。 |
| `interface.brandColor` | 界面展示色。 | 作为业务状态、风险等级或质量证据。 |
| `interface.composerIcon` / `interface.logo` | 指向插件 assets。 | 指向外部 URL、系统路径或动态生成内容。 |
| `privacyPolicyURL` / `termsOfServiceURL` | 指向 Analytix 官方隐私和服务条款。 | 指向空值、本地路径或非受控页面。 |

`.mcp.json` 字段契约：

| 字段 | 施工契约 | 禁止 |
| --- | --- | --- |
| `mcpServers.analytix_funds.command` | 使用受控运行时可解析的 Node 启动方式。 | 使用 shell 包装绕过环境隔离。 |
| `cwd` | 保持插件包内相对工作目录。 | 指向系统 Codex、用户 home 或本机临时绝对路径。 |
| `args` | 指向插件内 `mcp/server.mjs`。 | 指向开发脚本、eval 脚本或外部 server。 |
| `env_vars` | 只 allowlist 必要运行变量：`ANALYTIX_API_BASE_URL`、`ANALYTIX_API_TOKEN`、`ANALYTIX_FUNDS_ARTIFACT_DIR`、完整发现/debug 开关。 | 写死 token、backend 地址、系统路径或 provider 配置。 |
| `startup_timeout_sec` | MCP server 启动等待上限。 | 用过长等待掩盖启动失败。 |
| `tool_timeout_sec` | 单工具调用上限。 | 让重任务无界运行。 |
| `default_tools_approval_mode` | 默认工具审批策略。 | 将重写报告、全案长任务或危险路径设为无边界自动执行。 |
| `tools.run_full_case_analysis.approval_mode` | 全案写报告类重任务需要更强显式意图或审批。 | 让报告写入、长任务、全案材料生成在无明确意图下自动执行。 |

`Write` 能力边界：

- 允许写 Analytix-owned report draft、artifact、trace、eval output、Hub package source 和 runtime cache 中受控插件文件。
- 不允许写系统 Codex home、系统 hooks、系统 MCP config、全局 cache、案件数据库、后端配置、provider token 或用户非插件文件。
- 写入动作必须能被 doctor、release-slice audit、runtime-cache contract 或 Hub package 校验。
- 用户没有明确报告、导出、包准备、remount 或 eval 输出需求时，不应触发写入型重任务。

Doctor manifest ingestion 细则：

| 检查项 | 通过条件 | 阻断原因 |
| --- | --- | --- |
| 版本字段 | `version` 必须是 strict semver。 | 版本格式不可解析会导致 Hub package、runtime cache 和质量证据身份无法对齐。 |
| manifest key | 只允许插件摄取支持的 manifest key。 | 未知 key 可能被误认为运行配置或系统 Codex 配置。 |
| skills path | `skills` 必须指向插件内已存在的相对 skill 目录。 | 指向外部路径或系统 skill 会破坏隔离。 |
| mcpServers path | `mcpServers` 必须指向插件内 `.mcp.json`。 | 内联或指向全局 MCP 配置会污染系统状态。 |
| MCP server | `.mcp.json` 必须定义 `analytix_funds`。 | MCP server 身份不稳定，工具和 skill 无法绑定。 |
| interface required fields | `displayName`、`shortDescription`、`longDescription`、`developerName`、`category` 必须齐全。 | 插件页展示和 marketplace 摄取不完整。 |
| default prompt | `interface.defaultPrompt` 或等效字段必须是 1-3 条非空入口提示。 | 默认提示为空、过长或承载内部测试术语。 |
| capabilities | `interface.capabilities` 必须是非空字符串数组。 | 插件读写交互能力无法被 marketplace 正确摄取。 |
| URL 字段 | author、homepage、repository、privacy、terms 等 URL 必须使用 `https`。 | 本地路径或非受控地址可能泄露运行状态。 |
| brand color | `interface.brandColor` 必须是 `#RRGGBB`。 | 展示元数据不符合 ingestion contract。 |
| assets path | composer icon 和 logo 必须指向插件内 assets。 | 指向外部 URL、系统路径或缺失文件。 |

Doctor ingestion 不是产品质量证明，只证明插件包可被正确识别、安装和挂载。产品质量仍由事实层、证据账本、claim review、eval 和 runtime health 证明。

技能展示与能力可发现性契约：

- 插件早期可以保留一个兼容 root skill。root skill 是渐进披露入口，不等于插件只能有一个能力。
- 顶级商业成熟形态必须有 Data Analytics 式 focused skill 产品面；但不要求为了模拟多命令插件而拆出大量浅 skill。只有当某个能力具备独立触发条件、独立参考资料、独立脚本、独立交付面或独立评测面时，才拆成专项 skill。
- `/analytix` 命令族、能力矩阵、工具可用性、领域 playbook 和反模式规则必须通过 focused skills、`command-metadata.json`、`command-router.md`、Capability Registry、MCP resource、插件界面能力描述和默认提示被发现；命令数量以 registry/metadata 为准。
- 插件页只显示一个 skill 在 MVP 阶段不自动构成失败；但进入商业成熟验收时，若没有 `account-dossier`、`subject-dossier`、`full-case-analysis`、`fund-tracing`、`report-builder`、`claim-review` 等任务入口，则属于产品面未完成。
- root/index skill 不能变成长说明书；能力可发现性由短入口 + focused skills + references + resources + command metadata 承担。
- 多 skill 不是质量证明；真正质量证明是用户问题能自动路由到正确能力、返回事实结果、写清证据边界，并通过真实任务回归。

## 2. 产品主线与系统世界观

### 2.1 产品主线

```text
涉案资金事实内核
+ 侦查研判工作流
+ 研判输出准则 / 术语状态 / 反模式检测 / Critique Pass
+ 证据复核门禁
+ Command Router / Capability Registry
+ Casegraph / Fundgraph
+ Context Compiler
+ Claim Verifier
+ 研判事实卡 / 报告卡 / 续调清单
```

### 2.2 涉案资金事实内核

| 事实类型 | 内容 |
| --- | --- |
| 数据来源事实 | 导入文件、清洗状态、索引覆盖、字段质量、可分析范围。 |
| 主体事实 | 户名、人员、单位、证件/组织信息、直接账户、候选账户。 |
| 账户事实 | 账号、卡号、开户/销户、账户状态、账户类型、银行、余额口径。 |
| 交易事实 | 时间、金额、方向、对手方、摘要、余额、交易标识、source refs。 |
| 聚合事实 | 收入、支出、往来、Top、笔数、最大单笔、时间范围、覆盖口径。 |
| 图谱事实 | casegraph 节点、fundgraph supported edge、同事实族、质量门。 |
| 证据事实 | Evidence Ledger、Evidence Pack、Claim support、人工确认事实。 |

### 2.3 侦查研判工作流

```text
用户问题 / `/analytix` 命令
-> Command Router
-> Task Profile
-> Intent AST
-> Plan DAG
-> Deterministic Tool Runtime
-> Evidence Ledger
-> Context Compiler
-> Answer / Investigation / Flow / Report Card
-> Critique Pass / Claim Verifier
-> 用户答复、报告、附件、续调清单或阻断说明
```

### 2.4 证据复核门禁

| 门禁 | 覆盖对象 |
| --- | --- |
| Scope Gate | 当前案件、来源、清洗、索引、coverage、scope difference。 |
| Fact Gate | 金额、笔数、账户、户名、对手方、单位、时间窗、去重口径。 |
| Edge Gate | 资金边、下一跳、多跳、Mermaid、fundgraph。 |
| Hypothesis Gate | 假设状态、支持事实、反证、降级、禁止事实化。 |
| Claim Gate | 报告 claim、法律敏感词、候选归属、现金/资产端过度推断。 |
| Delivery Gate | 报告、附件、续调清单、图谱和复核材料。 |

### 2.5 端到端责任分工

| 层级 | 负责内容 | 不负责内容 |
| --- | --- | --- |
| Codex / Agent | 理解用户问题、提出侦查假设、选择下一步追问方向、组织专业表达；语义工具不足时可提出受控 workbench 计算请求。 | 不把未验证计算、弱来源、bounded preview 或样本行写成案件事实。 |
| Skill | 触发插件、说明硬边界、加载必要 reference、保持渐进披露。 | 不承载长 prompt，不塞入全量领域文档，不替代事实工具。 |
| Command Router | 把自然语言和 `/analytix` 命令映射到稳定能力族。 | 不作为工具菜单，不让模型逐个试工具。 |
| MCP Runtime | 执行确定性事实查询、聚合、路径、质量、复核和 artifact 管理。 | 不输出无证据结论，不把 raw payload 默认交给模型。 |
| Analytix Backend | 承载案件数据、权限、清洗、索引、聚合、交易事实和审计基础。 | 不被插件替代，不向 Agent 暴露裸数据库路径。 |
| Evidence / Context 层 | 记录证据来源、压缩事实、隐藏内部引用、生成门禁信息。 | 不把内部 id、raw query result dumps、全量 rows 暴露给用户。 |
| 输出门禁 | 审查事实边界、假设分级、报告 claim、Mermaid 边、禁用法律表述。 | 不替代人工最终确认，不把线索升级为法律事实。 |

当前案件上下文责任：

- analytix 案件项目 UI 中的选中案件必须同步到 current case-project binding，再由 MCP runtime 读取；不能让用户在案件中心已选择案件后，仍在 Agent 会话中被要求再次提供 `case_id`。
- `case_id` 显式输入、current case-project binding、Agent runtime 当前工作上下文三者冲突时，以显式 `case_id` 为最高优先级，其次 current case-project binding；冲突必须写入 scope warning，不能静默跨案取数。
- current case-project binding 为空但 UI 有选中案件时，主系统应在启动、进入 Agent、案件切换和 runtime bootstrap 时静默激活当前案件。
- 当前案件解析失败时，只允许输出阻断原因、同步动作和用户可执行下一步；不得假设默认案件或默认主体。

## 3. 产品能力全景

### 3.1 能力域

| 能力域 | 一等能力 |
| --- | --- |
| 案件与口径 | 当前案件识别、数据范围复核、清洗与覆盖、统计树/父子级聚合口径。 |
| 主体与账户 | 主体画像、一人一档、单账户画像、Top 排名、对手方研判。 |
| 资金链路穿透 | 来源追溯、去向追踪、多跳穿透、回流候选、混同资金、现金桥、金融产品、资产端、支付通道、补调闭环。 |
| 开放式研判 | Investigation Lab、假设队列、金额集中、时间集中、专题线索、多轮追查状态。 |
| 数据质量复核 | 重复流水、换卡、同事实多账户、空户名、缺失对手方、单位漂移、source audit。 |
| 专题线索 | 工程/项目款、涉诉/执行/扣划、工资/报销/差旅/劳务、对公向个人、资产消费、虚拟资产、支付通道。 |
| 图谱与上下文 | casegraph、fundgraph、Context Compiler、Evidence Ledger、Evidence Pack。 |
| 报告与附件 | 报告生成、报告改写、claim 复核、附件生成、续调清单、QA Review。 |
| 治理与边界 | 输出准则、反模式、Critique Pass、passive 非资金、quality eval、doctor、runtime health、Rust 边界。 |

### 3.2 一等能力规格

每项能力都按同一规格定义：

| 规格项 | 说明 |
| --- | --- |
| 能力定位 | 该能力解决什么业务问题。 |
| 适用问题 | 用户以什么方式触发该能力。 |
| 运行逻辑 | 该能力从意图到事实、证据、输出的主路径。 |
| 输出形态 | Answer Card、Investigation Card、Flow Card、Report Card、QA Review、Continuation List 等。 |
| 证据门禁 | 哪些事实、边、claim 必须复核。 |
| 降级规则 | 证据不足、scope 不明、字段缺失时如何降级。 |
| 成熟标准 | 能否支撑真实案件、真实追问和正式材料交付。 |

### 3.3 能力清单

| 能力 | 核心输出 | 关键门禁 |
| --- | --- | --- |
| 当前案件与范围识别 | Case Scope Card、scope warnings、可用能力建议。 | 无选定案件、后端 API 不可用、scope 不明时停止报告级结论。 |
| 数据来源、清洗与口径复核 | QA Review、Scope Difference Card、Data Quality Card。 | schema 暂不可用不能写成缺表；source audit 不足不能写“全量覆盖”。 |
| 统计树与聚合口径 | Aggregate Fact Card、Scope Mapping Card。 | Top、最大、全量、覆盖必须绑定 metric、scope、coverage。 |
| 主体画像 | Subject Profile Card、direct account list、candidate boundary。 | candidate account 不能写成名下账户；控制/代持只能写线索。 |
| 一人一档 | One-person Dossier、Subject Report Card、附件表。 | 所有金额、账户、对手方、可疑特征进入 Evidence Ledger 和 Claim Review。 |
| 单账户画像 | Account Profile Card、Top Counterparty Card、Behavior Card。 | 账户 totals 来自 aggregate/profile；bounded rows 只能作为例证。 |
| Top 排名 | Ranking Card、metric/scope/coverage/unit。 | 不能用 profile、dashboard metadata 或样本行推断全案 Top。 |
| 对手方研判 | Counterparty Card、Follow-up Target Card。 | 缺失对手方不能补想象主体；账号型对手方需要补调。 |
| 数据质量问题发现 | Data Quality Card、Duplicate Family Card。 | 全案盲去重禁止；重复候选不能直接扣减 totals。 |
| 续调清单 | Continuation List、Evidence Request Table。 | 附件级清单必须字段完整、理由清楚、金额/风险/缺口排序。 |
| 报告生成与改写 | Report Card、Finding Card、Draft Report、Attachment Table。 | Claim Verifier 通过前不能作为正式报告交付。 |
| 报告 claim 复核 | Claim Review Card、Adversarial Review Card。 | unsupported claim、unsupported flow、法律敏感词强结论必须阻断或降级。 |
| 附件生成与校验 | Attachment Table、QA Review。 | 行数、金额、单位、scope、导出字段必须可校验。 |
| 多轮追查状态 | Case Journal、Hypothesis History、Next Actions。 | 只能本案内复用，不能跨案记忆。 |
| 被动非资金边界 | 普通 Codex 答复或轻量说明。 | near-miss prompt 不能误触发案件事实工具。 |

### 3.4 领域能力细目

领域能力必须进入插件主能力，而不是只留在参考资料中。插件输出要像资金研判材料，必须能识别事实特征、线索特征、证据缺口和禁止外推边界。

| 领域能力 | 识别对象 | 输出形态 | 禁止外推 |
| --- | --- | --- | --- |
| 开户与账户登记事实 | 户名、证件号、账号/卡号、开户行、账户类型、开户/销户、账户状态、联系方式/地址等可用字段。 | Account Registry / Identity Scope Card。 | 开户事实写成实际控制、交易目的或资金流事实。 |
| 联系方式与住址关联 | 联系电话、住址、单位、法定代表人、同址/同电话/同单位重合。 | Identity / Contact / Address Lead Card。 | 同电话、同住址、同单位写成实际控制、代持、共犯或资金流事实。 |
| 设备/IP/MAC 与交易环境关联 | IP、MAC、柜员号、网点、地点、凭证/终端、商户/渠道字段、同设备/同环境重合。 | Transaction Environment / Device Lead Card。 | 同 IP/MAC、同柜员、同网点、同商户写成实际操作人、控制人、团伙或同一物理人。 |
| 调单反馈与查控措施 | 成功/失败反馈、查询对象、反馈缺口、冻结/扣划/查控措施、起止时间、金额。 | Task Feedback / Coercive Measure Card。 | 失败反馈写成无账户；查控措施写成犯罪事实或非法所得。 |
| 账户结构研判 | 直接账户、候选账户、同户名账户、换卡/补卡候选、未登记户名、空户名字段。 | Account / Holder Scope Card。 | 候选账户写成名下账户；空户名写成无主体。 |
| 主体资金画像 | 收入、支出、往来、沉淀、账户角色候选、Top 对手方、交易频率。 | Subject Profile / Ranking Card。 | 资金角色候选写成法律身份。 |
| 账户角色分类 | 生活卡、经营用个人卡、中转/过账、汇聚/归集、分发/代付、现金重、理财重、资产消费、 dormant/sudden-active。 | Account Role Candidate Card。 | 账户角色候选写成犯罪角色、法律身份或最终控制关系。 |
| 生活卡与正常性复核 | 工资、日常消费、水电房租、教育医疗、食品餐饮、稳定小额支出。 | Normality / Downgrade Card。 | 正常生活特征无矛盾时强行写成可疑。 |
| 中转/汇聚账户 | 多来源集中、快速转出、低余额沉淀、多人小额入账、一源多出、时间集中。 | Transit / Convergence Lead Card。 | 通道特征写成最终受益、洗钱或跑分结论。 |
| 交易行为特征 | 大额、频繁、拆分、同金额集中、时间集中、重复交易号、摘要关键词。 | Pattern Card / Lab Card。 | 不规律特征写成犯罪事实。 |
| 资金来源特征 | 前序入账、赎回、借贷、工资、工程款、对公来源、缺失来源。 | Source Trace Card。 | 可见来源写成最终来源。 |
| 资金去向特征 | Top 出账、下一跳、终点分类、无下游端点、补调对象。 | Destination / Flow Card。 | 可见停止点写成最终流向。 |
| 现金线索 | 取现、存现、柜面、ATM、同日/次日近额候选。 | Cash Break / Cash Bridge Candidate Card。 | 候选现金桥写成同一物理现金。 |
| 金融产品线索 | 理财、基金、证券、保险、申购、赎回、分红、再转出。 | Financial Product Flow Card。 | 金融产品交易写成资产归属或现金流向。 |
| 支付通道线索 | 支付宝、财付通、微信、银联、网联、支付机构账号型对手方。 | Payment Channel Card。 | 通道线索写成平台账户控制关系。 |
| 商户/平台/虚拟资产线索 | 商户号、商户名、支付机构、OTC/交易所/钱包/虚拟资产出入金关键词。 | Platform / Virtual-Asset Lead Card。 | 写成确认平台控制、虚拟资产转移、赌博结算、洗钱或地下钱庄。 |
| 资产消费线索 | 房、车、装修、物业、停车、保险、借贷、消费大额付款。 | Asset Lead Card。 | 资产消费写成资产权属。 |
| 境外/跨境资金线索 | 外币、境外银行/汇款、外贸/结售汇摘要、离岸关键词、跨境支付、虚拟资产出入金线索。 | Cross-Border Lead Card。 | 线索写成确认境外转移、地下钱庄或虚拟资产结论。 |
| 债务与代持线索 | 借款、还款、担保、代付、共用账户、控制候选、关联主体往来。 | Control / Debt Lead Card。 | 代持、实际控制、债权债务写成确认事实。 |
| 专题线索 | 工程/项目、涉诉/执行/扣划、工资/劳务/报销、对公向个人。 | Topic Lead Card。 | 交易目的、法律性质和涉案归属过度推断。 |
| 对公向个人两层统计 | 限定主体向个人付款总计、可识别工资/薪资/劳务/报销/差旅/补贴子集、未识别余额。 | Public-to-Private Two-Layer Table。 | 把总计都写成工资报销；双边流水重复计数。 |
| 正常成本与利益输送分层 | 主材、劳务、物流、税费、贷款还款等正常成本外观，与核心人员/关联公司/资产端交叉点。 | Cost Normality / Benefit-Transfer Lead Card。 | 把所有项目成本都写成利益输送，或忽略成本账户后续交叉。 |
| 证据补调 | 银行、支付机构、回单、合同、发票、资产登记、平台资料。 | Continuation List / Evidence Pack。 | 补调目标写成已证明事实。 |
| 团伙/共同控制关联 | 共同账户、共同对手、同电话/同住址、同 IP/MAC/设备、同网点/商户、同步交易窗口。 | Group Association Lead Card。 | 关联线索写成黑恶团伙、共同犯罪、实际控制或跑分组织。 |

领域措辞规则：

- `事实` 只用于工具返回且 ledger 支撑的金额、笔数、账户、户名、对手方、交易边。
- `线索` 用于规则命中、关键词命中、现金桥候选、资产消费候选、控制/代持候选。
- `假设` 用于需要进一步查证的侦查方向。
- `需复核` 用于 source、coverage、重复、换卡、口径、人工确认不足的内容。
- `禁止` 用于无证据法律定性、最终归属、确认控制、确认资产权属、确认同一现金。

### 3.4.1 任务分型软合同与风险升级

插件必须像 Data Analytics 一样先理解用户任务，再选择 focused skill、source-backed 路径和交付形态。任务档位是轻量分类器，不是硬路由、硬停止或工具预算封顶；它帮助 Codex 判断“默认低成本回答是否足够”，并在风险信号出现时升级到 quality、workbench、critique 或 report/visual delivery。

不能把“简单入口”误解成“简单回答”。自然语言越短，越需要用领域语义判断其真实工作量：`某卡/某人资金研判` 是对象画像，不是 quick fact；`全案分析` 是完整工作包，不是计划；`A 转给 B 多少钱` 是金额 claim 和口径核验，不是普通对手方排名。

| 任务档位 | 用户触发语义 | 正确工作量 | 典型 skill | 失败信号 |
| --- | --- | --- | --- | --- |
| Quick Fact | “某笔多少钱”“谁最多”“Top1”“这个账号进账多少”这类单点事实。 | 先用一个最小充分 source-backed 路径；若结果是 amount claim、存在重复/换卡/账户归属/join 风险、或工具标记 `needs_review`，升级一次针对性核验；无风险时给口径、金额/笔数、范围和边界。 | `quick-fact` | 把金额 claim 当排行行朗读；无风险也扫全案；有风险仍硬停。 |
| Pair Amount / Amount Challenge | “A 转给 B 多少钱”“这笔是不是重复”“金额范围对吗”“为什么是某个高金额统计范围”。 | 作为两方金额核验材料处理：必须确认 source-of-truth、holder/account scope、counterparty grain、raw/effective/dedup 口径、时间窗、重复/换卡风险、账户/时间集中、关系/角色线索和下一跳/Top 对手方核验；语义工具不足时进入 workbench 聚合。 | `pair-amount-investigation` / `data-quality` / `case-workbench` | 只用 `rank_counterparties` 排名金额作最终事实；只输出一个裸金额；把 raw duplicate total 写成确定金额；让 `funds_investigate` 或事实卡直接成稿。 |
| Ranking | “Top20”“排名”“最大出账/入账户名/对手方”。 | rank 工具为主，必须带 metric、direction、time window、success filter、coverage；ranking 可以回答排名，但不能自动升级为报告级 amount/path claim。 | `quick-fact` / `case-context` | 用 bounded rows 或画像结果推 Top；缺少口径；ranking 行被写成资金链事实。 |
| Object Dossier | “某卡资金研判”“某人/某公司资金研判”“画像”“基本情况”“异常特征”。 | 完整对象画像包：开户/登记事实、scope、账户结构、时间跨度、进出账金额笔数、Top 对手方、账户角色、生活卡/中转/汇聚等正常性与可疑特征、资金来源/去向、证据缺口、下一步追查。 | `account-dossier` / `subject-dossier` | 把对象研判压成普通问数；只答一个排名或一句摘要。 |
| Counterparty Analysis | “共同对手”“资金关联通道”“与谁往来密切”“对手方性质”。 | 对手方 Top、共同往来、账户型/户名型边界、缺失对手方、补调优先级。 | `counterparty-analysis` | 把对手方候选写成确认关系；不区分缺失/账号型对手。 |
| Trace / Continuation | “钱从哪里来/到哪里去”“继续追一层”“下一跳”“回流”。 | 沿 supported edge 或候选断点继续追；输出交易边、停止点、候选线索、补证动作。 | `fund-tracing` | `answer_card_complete` 后拒绝继续；把候选路径画成确定链。 |
| Investigation Lab | “全面梳理可疑点”“开放深挖”“异常特征跑一遍”。 | 假设队列、事实 probe、证据状态、禁止事实化、下一步追查队列。 | `investigation-lab` | 只列工具能力；没有可执行追查动作。 |
| Full Case Analysis | “全案分析”“完整研判”“按功能树跑完”“示例案件全案分析”。 | 全案分析树：数据量、时间跨度、清洗/开户/账户覆盖、进出账金额笔数、按人/账户统计、每人名下账户、账户角色分类、进出 Top20、现金/资产/理财/支付/境外等可疑特征、资金链路、专题线索、续调清单。 | `full-case-analysis` | 只给摘要；误入 report gate；没有数据盘点和分主体统计。 |
| Report Builder | “生成报告”“正式材料”“报告初稿”“附件”“一人一档”“专题研判”“续调清单”。 | 先选择报告形态：全案汇报材料、一人/一企一档、工程/职务犯罪/串标专题、资金去向追踪及续调清单、单线索核查；再汇总已验证 facts 写材料；报告 claim、附件和 Mermaid 必须复核。 | `report-builder` | 套用通用 8 段模板；报告新增未验证事实；普通研判被报告门禁污染；公安经侦材料写成工具说明或数据分析说明书。 |
| Visual Evidence | “做表格/图表/看板/附件”“Top20 图”“证据表”“导出工作包”。 | 从已复核事实生成证据表格、排序/集中度图、时间图、矩阵、看板卡、附件目录，并写清 scope、unit、time window、metric、direction、evidence status。 | `visual-evidence` | 图表制造新事实；把排行、矩阵、看板当成交易路径、控制关系或法律结论。 |
| Claim / QA Review | “核准口径”“复核报告”“检查金额/链路/附件”。 | 复查金额、笔数、单位、scope、unsupported flow、法律敏感措辞。 | `claim-review` / `data-quality` | 只返回 `write_blocked`，不给纠正口径和补证动作。 |
| Passive | 用户明确非案件资金任务，或要求不用插件。 | 不调用案件工具，完整回答普通任务。 | 无 | 非资金任务误触发案件工具。 |

对象画像和全案分析是本插件商业价值的核心：通用问答往往会把“某卡/某人资金研判”简单化，插件必须自动补齐完整研判包；人工多轮才完成的“全案分析”，插件必须把这类多轮压缩为可审计的全案工作流。但这不意味着每个短问题都触发重型流程；性能原则是 Data Analytics 式“轻量默认、风险升级、交付前强验证”。

### 3.4.2 多 skill 产品面合同

Analytix 采用 Data Analytics 式多 skill 产品面。每个 focused skill 必须是“任务入口”，不是“工具说明书”；每个 skill 的 `SKILL.md` 保持短入口和渐进披露，详细领域知识放入 references，确定性动作放入 MCP 或 scripts。

| Skill | 用户可见定位 | 触发语义 | 主要命令 | 共享 MCP 能力 | 完成标准 |
| --- | --- | --- | --- | --- | --- |
| `index` | 插件总入口和路由。 | `@Analytix`、`/analytix`、宽泛“能做什么/帮我分析”。 | `/analytix`、`/analytix ask` | `get_current_case`、`get_case_scope_map`、`funds_investigate` navigator | 给当前案件、可用 lane、下一步建议；不跑全案/报告。 |
| `case-context` | 当前案件、数据范围、语义索引预检。 | 当前案件、数据量、时间跨度、导入清洗覆盖、scope。 | `/analytix scope`、`/analytix map`、`/analytix casegraph` | `get_case_scope_map`、`get_scope_coverage`、`get_casegraph`、`audit_case_data_quality` | 输出 Case Scope Card、coverage、质量边界和后续可选能力。 |
| `data-quality` | 导入清洗、重复、换卡、缺失对手、口径复核。 | 数据质量、重复流水、换卡、同事实、空户名、口径不一致。 | `/analytix audit`、`/analytix duplicates`、`/analytix compare-scopes`、`/analytix qa` | `audit_case_data_quality`、`resolve_duplicate_families`、`compare_analysis_scopes` | 输出 QA Review；阻断不可靠 totals 和报告级结论。 |
| `quick-fact` | 普通单点事实、Top、排行快答。 | 谁最多、TopN、某账户/某户名金额笔数、低风险单点统计。 | `/analytix rank`、`/analytix ask` | `rank_accounts`、`rank_holders`、`rank_counterparties` | 低风险排行一轮给事实、口径和边界；遇到金额 claim、重复/换卡、账户归属、join 或对手粒度风险时转交对应 focused owner，不用 `funds_investigate` answer draft 成稿。 |
| `pair-amount-investigation` | 一跳两方金额核验材料。 | A 转给 B 多少钱、金额争议、重复/换卡/同事实、排行金额与明细核算不一致。 | 自然语言金额核验、`/analytix ask`、可选 `/analytix amount` | `get_current_case`、`get_case_scope_map`、候选 rank、`audit_case_data_quality`、`compare_analysis_scopes`、`run_case_sql`、必要时 `get_evidence_pack` | 作为 lead workflow owner 输出经侦小研判：结论、期间、笔数/金额、大额交易表、raw/effective/duplicate 口径、账户/时间集中、异常特征、关系/资金目的线索、下一步核验；MCP/card/workbench 只做 support。 |
| `account-dossier` | 某卡/某账户完整资金研判。 | 某卡研判、账号画像、账户基本情况、账户异常特征。 | `/analytix account <account>`、`/analytix destination` | `analyze_account_full`、`rank_counterparties`、`trace_subject_top_outflows`、`hypothesis_probe` | 输出账户资金画像：开户/登记、开户主体联系电话/住址线索、收支、重点对手方、账户角色、行为、异常、来源/去向、缺口；联系/住址只作线索不升级为控制。 |
| `subject-dossier` | 某人/某公司一人一档/一企一档。 | 某人资金研判、主体画像、名下账户、关联账户。 | `/analytix holder <name>`、`/analytix owner <name>` | `analyze_holder_full`、`rank_accounts`、`rank_counterparties`、`hypothesis_probe` | 输出主体资金画像；区分登记账户、待核账户线索、账户角色和证据缺口。 |
| `counterparty-analysis` | 对手方和关联通道研判。 | 共同对手、主要往来对象、关联资金通道。 | `/analytix destination`、`/analytix outflows`、`/analytix payment` | `rank_counterparties`、`trace_subject_top_outflows`、`classify_missing_counterparty_business` | 输出对手方优先级、证据状态和补调对象。 |
| `fund-tracing` | 来源、去向、下一跳、多跳、回流。 | 钱从哪来/到哪去、继续追一层、资金链路。 | `/analytix path <seed>`、`/analytix flowgraph`、`/analytix outflows` | `trace_fund_next_hop`、`trace_fund`、`build_fund_flow_graph`、`validate_continuation_list` | 输出 supported edges、停止点、候选断点和补证清单。 |
| `investigation-lab` | 开放式假设和专题深挖。 | 可疑点、异常特征、现金/理财/资产/工程/涉诉/境外线索、IP/MAC/设备、商户平台、虚拟资产、票税合同、团伙关联、账户角色。 | `/analytix lab`、`/analytix discovery`、`/analytix probe`、`/analytix pattern`、`/analytix cash`、`/analytix financial`、`/analytix topic`、`/analytix crypto`、`/analytix asset`、`/analytix control`、`/analytix role`、`/analytix device`、`/analytix group`、`/analytix platform`、`/analytix tax` | `hypothesis_probe`、专题 detectors、`audit_case_data_quality`、targeted rank/trace | 输出假设队列、账户角色候选、交易环境/团伙/平台/票税线索、证据状态、禁止事实化和下一步追查。 |
| `full-case-analysis` | 全案分析树和系统研判。 | 全案分析、完整研判、跑完全部分析、按功能树跑完、刑侦/经侦专案完整梳理。 | 自然语言全案分析、`/analytix plan` 仅用于显式规划、`/analytix report` 中非写作分析阶段 | `run_full_case_analysis(write_report=false)` 为默认执行；`plan_case_analysis` 仅用于显式计划或阻断分期；必要时补 scope/rank/profile/trace/quality/lab | 输出全案事实树、分主体统计、账户角色、Top20、现金/资产/理财/支付/境外特征、资金链路和续调清单。 |
| `report-builder` | 正式报告准备与宿主发布门。 | 生成报告、正式材料、报告初稿、附件、一人一档、专题研判、资金去向追踪及续调清单。 | `/analytix report` | `run_full_case_analysis(write_report=false)`、`validate_report_claims`、`validate_continuation_list`、报告形态 schema；`write_report=true` 在宿主 PublicationReceipt pipeline 接通前固定阻断 | 先匹配报告形态并形成非发布材料；只有宿主有效 PublicationReceipt 才能原子发布，不新增未验证事实，不输出工具说明书。 |
| `evidence-request` | 补调、续调、取证和核实清单。 | 补调清单、取证清单、证据包、下一步核实、银行/支付/平台/资产/票税材料。 | `/analytix evidence` | `generate_followup_investigation_list`、`get_evidence_pack`、`validate_continuation_list`、targeted trace/risk tools | 输出可执行 Evidence Request List：对象、事项、字段/时间范围、证明目的、优先级和边界；请求材料不等于已取得证明。 |
| `analysis-critique` | 研判质量、路线和缺失角度复核。 | 当前回答/计划/全案结果是否机械、漏项、路由错误、证据边界弱，下一步应使用哪个 skill。 | `/analytix critique` | `get_scope_coverage`、`audit_case_data_quality`、`compare_analysis_scopes`、`get_evidence_pack` | 输出 pass/revise/reroute/block、缺失 lane、过度结论风险和下一步 focused skill；不生成新案件事实。 |
| `claim-review` | 报告 claim、图谱和附件复核。 | 核准金额、链路、Mermaid、法律敏感表述、附件清单。 | `/analytix qa`、报告复核类自然语言 | `validate_report_claims`、`validate_continuation_list`、Evidence Ledger | 输出通过/阻断/降级表述和最小补证动作。 |
| `delivery-qc` | 用户可见交付质量和经侦口吻复核。 | 回答机械、像工具说明、无研判意义、表格/图谱/报告不可用、公安经侦口吻不足。 | `/analytix critique`、发布前交付复核 | 已生成用户交付、support facts、claim-review 结果、视觉/报告 artifact | 检查案件研判 spine、经侦材料 plan、证据成熟度、事实表、异常特征、侦查判断点、补证动作、support 隐身；不得新增事实。 |
| `graph-visualization` | 案件图谱和资金图谱表达。 | 画图、Mermaid、资金流图、关系图。 | `/analytix flowgraph`、`/analytix casegraph` | `get_casegraph`、`build_fund_flow_graph` | 只画 supported edge 或明确 candidate 状态；图是索引不是证据替代。 |
| `visual-evidence` | 表格、图表、看板和附件工作包。 | Top20 表、特征表、流向表、续调表、证据包、图表、看板、Excel/CSV 式附件、visual/table QA。 | `/analytix visual` | 已复核 fact cards、rank/trace/full-case/lab/evidence outputs、`get_evidence_pack`、`validate_continuation_list`、`validate_report_claims` | 输出 reader-facing evidence tables/charts/dashboard/appendix inventory；排行、矩阵、看板不是交易路径；图表不得新增事实。 |
| `case-workbench` | 受控 SQL/notebook/custom 口径工作台。 | 语义工具不足、明确自定义口径、SQL、notebook、可回放计算、chart-ready extract。 | `/analytix sql`、`/analytix notebook` | `get_case_scope_map`、`inspect_case_schema`、`audit_case_data_quality`、`run_case_sql`、`create_case_notebook` | 当前案件、只读、清洗 `fc_*_norm` / `analysis_*`、限行、purpose、可回放；SQL 只能使用 schema `sql_name/sql_identifier`，中文 `display_name` 只作展示；不可用时输出专项核算缺口说明，不伪造计算结果。 |

多 skill 施工规则：

- `skills/` 下可以保留 `analytix-fund-analysis` 作为兼容总入口，但商业成熟形态必须新增 focused skill 目录，并让 manifest/UI 能发现这些能力。
- 每个 focused skill 的 frontmatter `description` 必须写清“Use when”和“不适用场景”，避免所有任务都落回总入口。
- 每个 focused skill 只引用一层 references，不复制整份蓝本；共同硬边界抽到 shared reference。
- 每个 focused skill 都必须映射到 `capability-registry.json`、`command-metadata.json`、doctor、functional eval task、真机功能验收 prompt 和输出污染阻断规则。
- 内部交付子流程可以像 Data Analytics 的 `report-to-pdf` 一样作为 sub-skill 或 reference 存在，但不必全部顶到用户技能列表。
- 经侦 typology、账户角色、生活卡、中转/汇聚、现金桥、资产理财、境外/跨境等先作为 `investigation-lab`、`fund-tracing`、`account-dossier`、`subject-dossier`、`full-case-analysis` 内的领域 lane 和命令能力承载；只有当它们形成独立触发语义、独立资料、独立评测和独立交付面时，才拆成新 skill。成熟度看工作包完成质量，不看 skill 数量。

### 3.4.3 命令、skill 与领域能力映射

`/analytix` 命令族保留 Impeccable 式共享词汇价值，但命令不能替代自然语言理解。自然语言触发和 slash 命令必须映射到同一 focused skill 和 capability，不得形成两套路线。

| 命令族 | Focused skill | 领域能力 | 工具/门禁重点 |
| --- | --- | --- | --- |
| `/analytix`、`/analytix ask` | `index` / `quick-fact` | 入口、普通事实问答、模糊导航。 | `funds_investigate` 只作 navigator；可转 targeted semantic tool。 |
| `/analytix scope`、`/analytix map`、`/analytix casegraph` | `case-context` | 当前案件、scope、casegraph、coverage。 | current case project、scope、coverage、质量边界。 |
| `/analytix audit`、`/analytix duplicates`、`/analytix compare-scopes`、`/analytix qa` | `data-quality` / `claim-review` | 清洗覆盖、重复/换卡、口径复核、质量门禁。 | QA Review、重复候选不能扣减 totals。 |
| `/analytix rank` | `quick-fact` | 排行、Top20、最大入账/出账/往来。 | rank tools、metric/scope/direction/time/success filter。 |
| `/analytix account <account>` | `account-dossier` | 单账户完整画像。 | `analyze_account_full`、Top 对手、异常特征、证据缺口。 |
| `/analytix holder <name>`、`/analytix owner <name>` | `subject-dossier` | 某人/某公司一人一档、账户集、候选边界。 | `analyze_holder_full`、直接/候选账户分层。 |
| `/analytix destination`、`/analytix outflows` | `counterparty-analysis` / `fund-tracing` | 资金去向、Top 出账、补调对象。 | `trace_subject_top_outflows`、终点分类、`validate_continuation_list`。 |
| `/analytix path <seed>`、`/analytix flowgraph` | `fund-tracing` / `graph-visualization` | 下一跳、多跳、回流、资金流图。 | supported edge、Mermaid 安全、trace tolerance 口径。 |
| `/analytix lab`、`/analytix discovery`、`/analytix probe`、`/analytix pattern` | `investigation-lab` | 开放假设、可疑模式、反证和深挖。 | hypothesis status、forbidden-as-fact、下一步队列。 |
| `/analytix device`、`/analytix group` | `investigation-lab` / `subject-dossier` | IP/MAC/设备/柜员/网点/商户重合、团伙/共同控制关联线索。 | shared environment 是线索；不得写成实际控制、同一操作人、团伙或共同犯罪。 |
| `/analytix cash`、`/analytix cash-bridge` | `investigation-lab` / `fund-tracing` | 现金断点、现金桥候选。 | 候选现金桥不得写成同一现金事实。 |
| `/analytix financial`、`/analytix asset`、`/analytix payment`、`/analytix crypto`、`/analytix platform` | `investigation-lab` / `counterparty-analysis` | 理财、资产消费、三方支付、商户平台、虚拟资产线索。 | 关键词/规则命中是线索；不能证明资产权属、平台控制、虚拟资产转移或最终流向。 |
| `/analytix tax` | `investigation-lab` | 票税、合同、发票、物流/服务、项目成本与回流线索。 | 无发票、税务、合同、物流、服务和账册证据不得定性虚开骗税。 |
| `/analytix topic`、`/analytix role`、`/analytix control` | `investigation-lab` / `subject-dossier` | 工程/涉诉/专题、账户角色、控制/代持候选。 | 角色/控制只能是候选，法律定性需人工和外部证据。 |
| `/analytix evidence` | `evidence-request` / `fund-tracing` / `claim-review` | 补调清单、续调清单、取证清单、证据包和下一步核实事项。 | 请求对象、字段/时间范围、证明目的、优先级和边界必须清楚；请求材料不得写成已取得证明。 |
| `/analytix critique` | `analysis-critique` | 复核当前研判、计划、回答或全案结果是否漏掉完成包、经侦 lane 或证据边界。 | critique 是路线建议和质量复核，不生成新事实、不替代 full-case、data-quality 或 claim-review。 |
| `/analytix visual` | `visual-evidence` / `graph-visualization` / `claim-review` | 证据表格、Top20 图表、特征表、资金流向表、看板卡、附件目录和 visual/table QA。 | 必须已有事实来源、scope、unit、time window、metric、direction、evidence status；Mermaid 仍由 graph-visualization 和 supported edge 约束，报告级视觉 claim 由 claim-review 复核。 |
| 自然语言“全案分析/完整研判/跑完整功能树” | `full-case-analysis` | 全案分析树执行。 | 默认 `run_full_case_analysis(write_report=false)`，不是只出计划。 |
| `/analytix plan` | `full-case-analysis` | 复杂全案计划、预算、lane。 | Plan DAG 只用于用户明确要计划、复杂全案分期或阻断恢复，不压普通问答。 |
| `/analytix report` | `full-case-analysis` / `report-builder` / `claim-review` | 全案分析、报告生成、claim 复核。 | 明确报告意图才写报告；`validate_report_claims` 必过。 |

### 3.4.4 公安经侦业务能力矩阵

Analytix 的领域能力不能只停留在“通用资金异常”。插件必须把公安经侦常见案类沉淀为 typology lens：它们用于启发资金特征、事实工具和补证方向，不用于自动输出法律定性。

| 侦查镜头 | 资金模式 | 必测特征 | 输出形态 | 禁止外推 |
| --- | --- | --- | --- | --- |
| 电诈 / 两卡 / 跑分 | 多来源入账、快进快出、支付通道、取现或转下游。 | 高频、短持有、拆分、缺失对手、支付机构、现金断点。 | Feature Table、Flow Card、续调清单。 | 从流水直接写成诈骗账户、跑分人员或犯罪团伙。 |
| 网络赌博结算 | 夜间/高频、支付机构、重复金额、多人小额往来。 | 时间集中、金额集中、通道对手方、虚拟资产/现金线索。 | Topic Lead Card、Payment Channel Card。 | 无平台/订单证据就定性赌博结算。 |
| 洗钱 / 地下钱庄 | 多层转移、回流、现金/外贸/支付通道、虚拟资产线索。 | 层化、回流、现金桥、金融产品、跨主体通道。 | Fund Flow Card、Cash Bridge Candidate。 | 从路径复杂直接认定洗钱或地下钱庄。 |
| 团伙/共同控制 | 多主体共享账户、对手方、IP/MAC、电话住址、商户渠道或同步交易窗口。 | group association、casegraph、交易环境、共同通道。 | Group Association Lead Card。 | 共享线索直接写成黑恶团伙、共同犯罪或实际控制。 |
| 非法集资 | 多人向个人/公司集中转入，周期性返还或滚存。 | 集中度、投资人式对手、规律返利、合同/宣传线索。 | 主体资金画像、补证清单。 | 无合同、宣传、投资人材料就写成吸收资金。 |
| 传销 / 会员返利 | 成员式入账、层级式出账、重复返利。 | 层级图、重复支付、团队/会员关键词。 | Casegraph Lead、Feature Table。 | 无会员/平台/通讯材料就写成层级组织。 |
| 职务侵占 / 挪用 | 公司或项目资金转个人/关联账户，报销工资摘要、资产端。 | 对公向个人、项目通道、报销/劳务、资产消费、控制候选。 | 主体资金画像、资产线索卡。 | 仅凭公司到个人流水认定侵占或挪用。 |
| 贿赂 / 腐败 | 项目审批窗口、同名/通讯录/关联人、资产或利益输送。 | 身份强弱匹配、时间窗口、资产端、关联人代收。 | Identity Lead、Claim Review。 | 同名或弱匹配写成公职人员、受贿对象或行贿事实。 |
| 虚开骗税 / 税案 | 公司间循环、发票/合同关键词、对公转私和现金回流。 | 闭环流转、项目公司通道、摘要关键词、税期聚集。 | Topic Lead Card、补证清单。 | 无发票、税务、合同、物流/服务证据就定性虚开。 |
| 虚拟资产 / OTC | 交易所、钱包、OTC、支付通道、跨境或现金转换线索。 | 平台/商户线索、IP/MAC、KYC 缺口、法币出入金。 | Platform / Virtual-Asset Lead Card。 | 无平台/KYC/钱包/链上证据就认定虚拟资产转移。 |
| 合同 / 项目 / 招投标 | 项目款来源、关联公司流转、个人账户承接、资产端。 | 项目关键词、关联通道、来源/去向、补调对象。 | Project Flow Card、Report Finding。 | 把所有项目款直接写成违法收益或利益输送。 |

每个 typology 输出都必须包含：feature family、supporting facts、evidence_status、downgrade reason、next proof。没有外部证据时，只能写为线索、假设、需复核、建议调取。

### 3.4.5 旧线程行为产品化合同

旧线程的价值不是它们上传文件或手工 SQL 的实现方式，而是人工多轮研判已经证明的侦查工作模式。Analytix 生产插件必须围绕当前案件 DuckDB/分析索引，通过 source-backed 能力层把这些模式压缩成少轮、可复核的产品能力：

| 旧行为模式 | 产品化能力 | 验收信号 |
| --- | --- | --- |
| 先确认导入、清洗、覆盖、字段、口径。 | 只读数据质量与 scope audit。 | 说明 source/import/clean/index/report scope，不执行导入清洗。 |
| 用户给关系、动机、外部线索后继续分析。 | 关系输入降级为 hypothesis / subject pool。 | 有 MCP 证据前不升级为事实。 |
| 用户说金额不对、换时间窗、换主体池。 | 金额质疑包与 rescope/recompute。 | 输出旧口径、新口径、可支持部分、不可支持部分、纠正表述。 |
| 用户问“有没有/是否出现”。 | 字段级 negative search。 | 输出已查字段和未命中边界，不写绝对不存在。 |
| 用户要求继续追一层。 | 默认 one-more-hop continuation。 | 给前手/后手、断点、金额/时间差、补调对象，不被上一轮完成状态压制。 |
| 技术探索变成研判报告。 | 事实卡 -> claim 抽取 -> 复核 -> 安全 wording -> 补证清单。 | 输出经侦材料语言，不输出 SQL、插件教程、内部 id 或 gate 模板。 |

### 3.4.6 案件 DuckDB 事实层与经侦完成包

Analytix 涉案资金研判不是“只查资金明细表”。当前案件 DuckDB/分析索引至少应被 Codex 理解为多层事实源：清洗交易明细、交易环境/IP/MAC/柜员/网点/商户、开户/账户登记、人员身份、联系电话、住址、调单反馈、查控措施、主体/户名索引、对手方索引、casegraph/关系层、管线与口径复核层。插件锁住的是这些事实层的访问边界，不能锁住 Codex 如何围绕这些事实提出研判假设。

生产研判事实必须来自 Analytix 数据清洗模块产出的 `fc_*_norm` 表、`analysis_*` 索引，以及 backend / controlled workbench / MCP 承载的可审计编译事实卡。`fc_*_raw`、原始文件、raw rows 和未复核计算只能属于导入/清洗模块、开发诊断或专项质量排查，不得作为普通 skill 或 source helper 回答用户资金问题的事实来源。

| 事实层 | 可支持内容 | 禁止 |
| --- | --- | --- |
| 清洗交易明细层 | `fc_transaction_norm`、`analysis_txn_detail_idx` 中的金额、笔数、方向、时间、账户、对手方、摘要、交易边、局部例证。 | 用 bounded preview 汇总全案 totals；回退 raw 表算事实。 |
| 交易环境/设备/渠道层 | 清洗交易索引中的 IP、MAC、柜员号、网点、地点、凭证/终端、商户名/商户号、支付渠道等线索。 | 同 IP/MAC、同网点、同柜员、同商户写成实际操作人、控制人、团伙或同一物理人。 |
| 开户/账户登记层 | `fc_account_norm`、`fc_sub_account_norm`、`analysis_account_dim` 中的户名、证件、开户行、账户类型、开户/销户、状态、子账户等身份与范围事实。 | 写成实际控制、交易目的、资金流或法律身份。 |
| 人员/联系方式/住址层 | `fc_person_norm`、`fc_person_contact_norm`、`fc_person_address_norm` 中的姓名、证件、单位、电话、住址、法定代表人等关联线索。 | 同电话/同地址/同单位写成控制、代持、共犯或资金流事实。 |
| 调单反馈与查控层 | `fc_task_success_norm`、`fc_task_fail_norm`、`fc_coercive_measure_norm` 中的反馈覆盖、失败缺口、冻结/扣划/查控措施。 | 失败反馈写成不存在；查控措施写成犯罪成立或非法所得。 |
| 主体/户名索引层 | 直接账户、候选账户、别名、同名、公司/个人标签、主体池。 | 候选账户和弱匹配升级为确认名下账户。 |
| 对手方索引层 | Top 对手、共同对手、账号型对手、缺失对手、通道/商户线索。 | 缺失对手写成隐藏身份；共同对手写成确认关系。 |
| casegraph/关系层 | 人-企-账户-电话-住址-线索关系、外部输入假设、关联路径。 | graph node/edge 替代 supported transaction edge。 |
| 管线与口径层 | import/source coverage、cleaned detail、analysis index、report scope、重复/换卡候选、未进入索引的清洗表缺口。 | 混用层级、静默去重或把候选重复扣减 totals。 |

三类核心完成包必须写入程序、metadata、functional eval 和真机功能验收：

| 完成包 | 必须覆盖 |
| --- | --- |
| 单卡/单账户研判 | 开户/登记事实、账户状态与类型、交易覆盖、时间跨度、进出账金额笔数、Top 对手方、账户角色分类、生活卡/中转/汇聚/现金/资产/理财/支付/境外线索、IP/MAC/柜员/网点/商户/渠道线索、来源/去向、证据缺口和下一步。 |
| 某人/某公司主体研判 | 身份与开户事实、联系电话/住址/单位/法定代表人关联线索、直接账户和候选账户边界、每账户角色、账户组合排行、主体进出账总额笔数、Top 对手方、共同通道、关联人/关联公司线索、IP/MAC/设备/商户/渠道重合、资产/现金/理财/境外线索、补调优先级。 |
| 全案分析 | 数据量、时间跨度、cleaned/index/report coverage、开户/人员/联系电话/住址/交易环境/IP/MAC/调单反馈/查控措施覆盖、人员/公司/账户清单、按人和按账户统计、每个重点主体名下账户、进出 Top20、账户角色分布、团伙/共同控制关联线索、现金同存同取、资产消费、投资理财、支付通道、商户平台、虚拟资产/OTC、境外/跨境、票税合同、工程/涉诉/工资报销/对公转私两层统计、正常成本与疑似输送分层、资金链路、否定性发现、补调清单和报告可用事实。 |

这三类任务的完成标准必须高于自然简单回答。用户一句“某卡/某人资金研判”不是 quick fact；用户一句“全案分析”不是计划请求；插件价值就在于把过去多轮挤牙膏式追问压缩成一次可审计的完整工作包。

### 3.5 Codex 可见语义事实工具箱与能力归属

Specialist tools 不能被理解成“默认隐藏、只由前门代调用”的后台菜单。对 Codex 来说，scope、casegraph、排名、账户画像、主体画像、对手方、资金穿透、假设探针和质量复核等高价值语义工具，是让它更会想的事实器官；低层 raw rows、debug payload、写入、导出和报告重链路才需要收进条件发现。生产默认工具面必须是“可审计语义事实工具箱”：数量受控、schema 清楚、输出压缩、证据边界明确，并允许 Codex 按用户问题自主选择。

默认认知工具箱至少覆盖这些语义事实能力：

- 当前案件与范围：`get_current_case`、`get_case_scope_map`、`get_casegraph`、`get_scope_coverage`。
- 排名与画像：`rank_accounts`、`rank_holders`、`rank_counterparties`、`analyze_account_full`、`analyze_holder_full`。
- 主体出账与链路：`trace_subject_top_outflows`、`trace_fund_next_hop`、`trace_fund`、`build_fund_flow_graph`。
- 开放假设与质量：`hypothesis_probe`、`audit_case_data_quality`、`resolve_duplicate_families`。
- 身份、联系方式、住址、交易环境/IP/MAC/商户/渠道和调单覆盖：现有 scope/profile/quality 工具必须编译清洗表事实；若事实卡缺失，优先补语义工具或扩展 `analyze_account_full`、`analyze_holder_full`、`get_case_scope_map`、`audit_case_data_quality`；明确自定义口径可临时走 Controlled Case Workbench，但不能把 raw/source 线索或未验证计算写成生产事实。
- 交付复核：`validate_continuation_list`、`validate_report_claims`。
- 自然语言导航：`funds_investigate` 只能作为 shortcut / navigator / route hint，不能成为替 Codex 思考和工具选择的总指挥。

| 能力族 | 受控工具 | 职责 | 使用边界 |
| --- | --- | --- | --- |
| 当前案件与状态 | `get_current_case`、`get_case_status` | 解析选中案件、案件状态、skill surface 和基础 dashboard metadata。 | dashboard counters 只能作状态信息，不作报告级 coverage。 |
| 导入、清洗与管线 | `get_import_overview`、`get_cleaning_overview`、`get_case_data_pipeline_overview` | 导入文件、清洗任务、stats pipeline、树状统计和管线状态。 | 只说明来源和管线状态，不能直接支撑最终 totals。 |
| 范围与 schema audit | `get_case_scope_map`、`inspect_case_schema`、`audit_unindexed_sources`、`audit_case_data_quality` | 低上下文 scope graph、schema inventory、未索引清洗来源、清洗质量、空户名、联系方式/住址覆盖、IP/MAC/柜员/网点/商户/渠道覆盖、调单成功/失败覆盖、查控措施覆盖、换卡和同事实候选。 | schema 暂不可用或 audit 需复核时只能写 coverage gap；raw/source 线索若用于导入/清洗诊断，不能直接升级为 production claim。 |
| 口径与覆盖 | `get_case_reconciliation`、`get_scope_coverage`、`compare_analysis_scopes` | detail、directional rows、tree、report scope、coverage 和统计范围差异。 | 报告级金额和笔数必须说明来自哪个 scope。 |
| 统计与 bounded rows | `get_stats_meta`、`get_stats_tree`、`query_stats_rows`、`query_stats_txn_rows`、`query_account_txn_rows`、`query_txn_slice` | 统计状态、交易时间范围、排行行、局部交易切片和例证行。 | bounded rows 只能作例证或局部事实，不能计算全案 totals。 |
| dashboard 与规则命中 | `get_analysis_dashboard`、`get_account_stats`、`get_rule_hits`、`scan_case_risks` | 趋势、结构、账户统计、规则命中、风险线索。 | dashboard metadata 不能冒充 coverage；规则命中默认是线索。 |
| 主体与账户解析 | `resolve_account_scope`、`resolve_holder_scope`、`resolve_owner_scope`、`resolve_duplicate_families` | 账户、户名、主体账户集合、直接账户、候选账户、联系方式/住址/单位关联线索、重复/换卡候选族。 | candidate-linked accounts、contact/address overlaps、duplicate candidates 不能升级为确认事实。 |
| 排名 | `rank_accounts`、`rank_holders`、`rank_counterparties` | 账户、户名、对手方确定性排名和低风险聚合线索。 | 必须带 metric、direction、scope、success filter、coverage、单位和 validation/use 状态；ranking 行不得自动成为 pair amount、supported path、报告 claim 或重复扣减依据。 |
| 账户与主体画像 | `analyze_account_full`、`analyze_holder_full`、`get_evidence_pack` | 单账户、户名集合画像、身份/开户/联系/住址关联、证据包。 | aggregate/profile 支撑 totals；bounded rows 只作证据样例；联系/住址只是关联线索。 |
| 开放式探针 | `hypothesis_probe`、`run_discovery_scan`、`trace_holder_destinations` | Agent 提出的假设、关键词、主体范围、身份/联系/住址/IP/MAC/商户/渠道关联、团伙/共同控制线索、专题发现、去向候选聚合。 | 必须有 hypothesis、scope、fact need；输出为 lead / evidence gap。 |
| 现金、金融产品与专题 | `detect_cash_breakpoints`、`detect_financial_product_flows`、`detect_project_litigation_asset_leads`、`classify_missing_counterparty_business` | 现金断点、理财/证券/保险、工程/涉诉/资产、缺失对手业务分类。 | 分类是线索或纠偏，不能证明最终流向、交易目的或资产归属。 |
| 主体出账与路径 | `trace_subject_top_outflows`、`trace_fund_next_hop`、`trace_fund`、`build_fund_flow_graph`、`get_casegraph` | Top 出账、下一跳、多跳、资金流图、casegraph/fundgraph 查询。 | 只有 supported transaction edge 能画 Mermaid；unmatched endpoint 是补调目标。 |
| 计划与全案报告 | `plan_case_analysis`、`run_investigation_lab`、`run_full_case_analysis` | lane plan、工具预算、开放式 Lab、全案报告工作流。 | 报告级输出必须覆盖 mandatory review cards 和 claim review。 |
| 复核与交付门禁 | `generate_followup_investigation_list`、`validate_continuation_list`、`validate_report_claims` | 补调对象生成、续调清单、附件、报告 claim、Mermaid、法律敏感词和来源边界复核。 | 未通过时只能输出阻断点、纠正口径和下一步验证动作。 |

语义事实工具箱规则：

- 工具存在不等于低层全暴露；默认必须暴露足够的高价值语义事实工具，让 Codex 能自主构造研判路径。
- 低层 raw rows、debug、写入、导出、全案报告重链路和专项 eval 工具只能由 capability lane、Plan DAG、recommended next action 或显式专项任务选择。
- `funds_investigate` 的建议只是 route hint；Codex 可以根据用户新对象、新口径、新时间窗、新线索继续选择其他语义事实工具。
- `rank_counterparties` 是排名工具，不是所有“A 转给 B 多少钱”的最终事实工具；当返回结果存在同事实、换卡、账户归属、对手方粒度或时间窗风险时，必须把结果标为 `needs_review` 或 `ranking_only`，并建议 targeted verification / workbench，而不是写“已可直接作答”。
- 工具返回的事实必须进入 Evidence Ledger 或由 Context Compiler 压缩后才能支撑用户输出。
- 工具失败、缺字段、暂不可用、bounded preview、candidate family 都不能被模型补成事实。
- 新增 specialist tool 必须同步 Capability Registry、command metadata、tool schema、doctor、eval、answer contract 和 reference 映射。

### 3.6 Data Analytics 式性能与上下文预算

Data Analytics 的成熟做法不是“每个问题都跑完整数据质量、完整 notebook、完整报告门禁”，而是用轻量 preflight、focused skill 和风险触发升级，把 token、工具调用和交付深度控制在任务所需范围内。Analytix 大改也必须遵守这个性能合同，避免把“source-backed”误实现成“每题全量验算”。

| 层级 | 默认行为 | 升级触发 | 性能要求 |
| --- | --- | --- | --- |
| Index / Navigator | 只做能力发现、意图归类、focused skill 选择。 | 用户意图模糊、跨 skill、需要报告/图表/附件。 | root/index 输出短，不加载全量领域 reference，不输出大 JSON。 |
| Case Preflight | 编译 `case_source_envelope` 的最小子集：当前案件、清洗范围、source-of-truth、已知风险。 | 金额争议、重复/换卡风险、跨表 join、报告/附件/正式材料。 | envelope 必须压缩，详细 rows、SQL、trace 放 artifact / `_meta` / ledger，不塞进模型上下文。 |
| Inline Quick Answer | 用一个最小充分 source-backed 路径回答低风险事实、排行或小范围计算。 | amount claim、同事实候选、holder/account 粒度不清、工具返回 `needs_review`。 | 通常 1-2 个语义工具；不默认全案 scan、不默认 report gate。 |
| Focused Workflow | 对象画像、资金追踪、全案分析、可视证据、报告等由 focused skill 拥有 workflow。 | 用户明确要求完整研判包、继续追一层、报告、图表、附件或开放深挖。 | 只加载对应 skill 与必要 reference；每个 workflow 输出 compact facts + evidence pointers。 |
| Workbench / Notebook | 语义工具不足且任务明确时进入受控 SQL/Python/notebook。 | 自定义口径、工具缺口、金额挑战、复杂 join/去重/重复候选核验。 | 只读、当前案件、清洗/analysis scope、限行、purpose、可回放；SQL 字段来自 `inspect_case_schema` 的 `sql_name/sql_identifier`，中文展示名不能直接执行；结果摘要进上下文，明细进 artifact。 |
| Data Quality / Critique | 针对已形成的 claim 做复核，不替代分析。 | 高风险金额、报告、Mermaid、图表、法律敏感词、用户质疑。 | 风险触发，不对每个普通排行做全套审计；失败给 blocker/gap，不生成模板噪声。 |

验收时不得把“工具调用越多、上下文越长、分数越高”当作成熟。真正的性能目标是：低风险问题轻量、风险问题补足验证、交付型任务真实产物、所有重型工作可回放且不污染普通用户输出。

## 4. 资金链路穿透专项模型

资金链路穿透是一等核心能力，不是单独图谱展示，也不是 Top 对手方查询。它把主体范围、账户范围、逐笔交易边、续查对象、终点分类、证据缺口和报告门禁组织成可复核的链路研判闭环。

### 4.1 能力范围

| 子能力 | 研判内容 | 输出 |
| --- | --- | --- |
| 资金来源追溯 | 从目标入账或转出后的反向追溯资金来源、前序购赎、余额承接、来源缺口。 | Source Trace Card。 |
| 资金去向追踪 | 从主体/账户出账找一跳去向、下一跳、终点分类、补调对象。 | Destination Card、Continuation Card。 |
| 多跳链路穿透 | 基于 supported transaction edge 构建可复核多跳链路。 | Flow Card、Fundgraph、Evidence Table。 |
| 回流/闭环候选 | 识别资金从 A 到 B 后再回到 A 或关联主体的候选路径。 | Loop Candidate Card。 |
| 混同资金研判 | 对混同账户后的资金承接做候选归因和可行性分析。 | Commingled Fund Card。 |
| 现金断点与现金桥 | 识别取现、存现、同日/次日金额接近候选、柜面/ATM/摘要特征。 | Cash Break / Cash Bridge Candidate Card。 |
| 金融产品承接 | 理财、基金、证券、保险申购、赎回、分红、再转出。 | Financial Product Flow Card。 |
| 资产端线索 | 购房、购车、装修、物业、停车、保险、借贷等资产消费方向。 | Asset Lead Card。 |
| 支付通道线索 | 微信、支付宝、财付通、银联、网联等通道出入金。 | Payment Channel Card。 |
| 补调闭环 | 账户、银行、支付机构、合同、发票、资产登记、平台资料。 | Continuation List。 |

### 4.2 穿透任务画像

| 问题类型 | 正确 lane | 输出内容 | 禁止 |
| --- | --- | --- | --- |
| 一跳金额 | destination | 某主体/账户向某对手方转出金额、笔数、日期范围、口径。 | 扩大成多跳图谱或报告结论。 |
| Top 出账去向 | destination / outflows | Top 对手方、金额、笔数、终点分类、补调优先级。 | 把 Top 聚合排行画成交易链。 |
| 下一跳追踪 | path | seed 后的可见下一跳、交易边、停止点。 | 用时间邻近或金额接近补画缺失边。 |
| 多跳链路 | path / flowgraph | supported edge 序列、Mermaid 索引、交易事实表。 | 没有逐笔边时画确定性箭头。 |
| 终点分类 | destination / pattern | 现金、理财、支付通道、资产消费、缺失对手方等分类线索。 | 把分类线索写成最终资金归属。 |
| 补调清单 | evidence / qa | 账户、银行、支付机构、材料、字段缺口、优先级。 | 未经 `validate_continuation_list` 就交付附件级清单。 |

### 4.3 穿透运行逻辑

```text
主体/账户/scope 解析
-> 口径确认：方向、时间窗、去重、成功/失败、单位
-> 一跳事实：金额、笔数、日期、对手方
-> 续查对象：账户型对手方、缺失对手方、现金/理财/资产/支付线索
-> 下一跳 / 多跳 tracing
-> supported edge 进入 fundgraph
-> 终点分类与补调动作
-> Evidence Ledger + Claim Verifier
-> Flow Card / Report Card / Continuation List
```

### 4.4 穿透流程

1. 范围解析：解析当前案件、主体、户名、账号、候选账户、时间窗、方向和 seed。主体范围必须区分直接登记账户、候选关联账户、显式账号集合和未知对象。
2. 口径确认：确认入账/出账/往来、成功/失败交易过滤、去重口径、账户集合、日期范围、金额单位和 coverage。任何 `Top`、`最大`、`主要去向` 都必须有排序指标和 coverage。
3. 一跳事实：用确定性聚合或交易事实给出 source -> counterparty 的金额、笔数、日期范围、交易摘要。一跳金额题如果已经满足用户问题，应完成本轮回答，不把本轮预算状态写成跨轮限制。
4. 续查对象生成：对 Top 出账对象、缺失对手方、账户型对手方、支付通道、现金断点、理财/资产关键词生成续查对象。续查对象是 follow-up target，不是最终流向。
5. 下一跳和多跳 tracing：对明确 seed 调用 `trace_fund_next_hop` 或 `trace_fund`。每跳必须有 account、counterparty、amount、time、direction、transaction id 或可替代事实字段、edge status、source scope。
   - `trace_fund_next_hop.amount_tolerance` 和 `trace_fund.tolerance_amount` 是相对比例，不是金额；`0.03` 表示 3% 容差，不表示 0.03 元。
   - 不确定容差时宁可省略或请求确认，不能把比例参数当成金额参数。
6. fundgraph 构造：只有 `edge_status="supported"` 的逐笔交易边可以进入 fundgraph 和 Mermaid。聚合排行、候选路径、关键词线索、现金桥候选不能进入确定性边。
7. fundgraph 事实包：模型可见输出必须同时给出上游金额/笔数/集中日、续查对象后续 Top 出账、可解释交易边、scope 覆盖、不能认定事项和补证方向；不得只返回 graph contract、edge count 或金额列表，否则真实续查会退回多工具拼材料。
8. 终点分类：将可见终点分成继续可见下游、现金断点、理财/证券/保险、支付机构/三方通道、资产消费线索、缺失对手方、外部不可见、需补调。分类只是证据状态，不是最终归属。
9. 证据复核：每个金额、笔数、交易边、终点分类、补调对象进入 Evidence Ledger。报告或附件级输出前，链路 claim 经 Claim Verifier，补调清单经 `validate_continuation_list`。
10. 输出编译：Context Compiler 只给模型事实卡：参数、直接事实、supported edges、候选线索、边界、下一步。raw rows、raw graph payload、内部 id 留在 artifact / `_meta`。
11. 收口边界：用户问题已由一跳事实回答、无 supported next-hop、超出可见数据、关键字段缺失、schema/coverage 待复核、Claim Verifier 阻断、补调材料缺失时，必须停止强结论。

### 4.5 穿透输出规范

资金穿透输出必须包含：

- 参数：case、holder/account、direction、time window、depth、tolerance、scope。
- 直接事实：金额、笔数、日期范围、source、counterparty、单位、去重口径。
- 链路事实：每跳交易边、金额、时间、from、to、edge status、停止点。
- 终点分类：现金、理财、支付、资产、缺失对手、外部不可见、需补调。
- 证据边界：缺失字段、候选账户、混同资金、现金隔断、未纳入口径来源、coverage 风险。
- 下一步：补调账户、银行、支付机构、流水期间、回单字段、合同、发票、资产登记、平台资料。

Mermaid 只能作为索引：

```text
Mermaid = supported transaction edges index
交易事实表 = 报告级核心证据
边界说明 = 防止模型过度解释
补调清单 = 下一轮穿透动作
```

### 4.6 混同资金、现金桥与资产端

混同资金：

- 不能宣称同一笔物理资金已确定转移。
- 必须展示 allocation model 或 candidate path、time gap、amount gap、balance feasibility、competing candidates、visible-data cut-off。
- allocator 未形成确定性契约时，只能写资金承接可能性和候选链路。

现金桥：

- 只能作为同日/次日取现-存现对应特征或同存同取特征。
- 必须包含取现账户、取现时间、取现金额、存现账户、存现时间、存现金额、时间差、金额差、渠道/网点、交易 id 或可替代事实字段。
- 因现金具有物理隔断属性，不能仅凭流水认定为同一笔资金。

理财、资产与支付通道：

- 关键词/规则命中只能写线索。
- 逐笔交易事实只能证明交易金额、时间、账户、对手方和摘要。
- 平台/银行补调回证、资产登记、合同、发票、询问笔录闭合后，才可支持更强综合判断。
- 插件不能替代最终人工法务判断。

### 4.7 穿透能力门禁

- 一跳金额题：一次 `trace_fund_next_hop`、`trace_subject_top_outflows` 或 `funds_investigate` navigator 可答，且不额外调用 graph / lab。
- Top 出账题：返回金额、笔数、对手方、scope、coverage、终点分类和补调对象。
- 下一跳题：只输出 supported next-hop；无下游时明确停止点。
- 多跳图谱题：Mermaid 所有边都能在事实表中找到对应交易字段。
- 现金桥题：只输出 candidate pair 和 caveat，不写同一现金事实。
- 理财/资产题：线索、交易事实、补证要求分离。
- 续查清单题：通过 `validate_continuation_list` 后才可作为附件级清单。
- 报告链路题：通过 `validate_report_claims`，unsupported flow 必须阻断或降级。
- 成本：普通穿透题默认由 Codex 选择一个最小充分语义工具；图谱/报告题只允许必要追加。

## 5. 开放式假设实验室

开放式假设实验室把 Codex 的开放式侦查思考组织成证据约束下的假设队列。它不能压制 Codex 的侦查思考，而是把自由探索变成证据约束下的开放式深挖。

### 5.1 能力定位

| 项 | 规格 |
| --- | --- |
| 能力定位 | 把开放式侦查思考组织成证据约束下的假设队列。 |
| 适用问题 | “继续深挖”“有没有异常”“围绕某主体找线索”“按侦查方式查问题”。 |
| 运行逻辑 | intent -> scope -> hypothesis_queue -> probe/trace/rank/classify -> status update。 |
| 输出形态 | Investigation Lab Card、Hypothesis Queue、Next Query Spec。 |
| 证据门禁 | 假设必须有 proposed/probing/supported/contradicted/partial/downgraded/closed 状态。 |
| 降级规则 | 未验证假设只能写线索、需复核、下一步查询。 |
| 成熟标准 | 能保留开放探索的敏锐度，同时降低幻觉和无证据结论。 |

### 5.2 假设生命周期

| 状态 | 含义 | 输出边界 |
| --- | --- | --- |
| proposed | Codex 或用户提出的侦查假设。 | 只能列为待验证方向。 |
| probing | 已进入确定性事实查询或专题探针。 | 必须写明正在查什么事实。 |
| supported | 有确定性事实支撑。 | 可以写入研判事实，但保留口径。 |
| contradicted | 被事实反证。 | 必须删除或写明不支持。 |
| partial | 有部分事实支撑，但链条不完整。 | 写成线索或部分支持。 |
| downgraded | 原强结论被降级。 | 只能写线索、需复核或补证方向。 |
| merged | 与其他假设合并。 | 保留合并原因和证据来源。 |
| closed | 达到收口边界或用户确认停止。 | 写明停止原因和未解决缺口。 |

### 5.3 Investigation Lab Card

```text
investigation_intent
verified_facts
hypothesis_queue
hypothesis_status
suspicious_groups
amount_concentrations
time_concentrations
duplicate_or_same_fact_risks
related_account_or_card_switch_risks
missing_counterparty_or_cash_breaks
wealth_management_or_asset_clues
continuation_paths
excluded_or_downgraded_hypotheses
next_queries
forbidden_as_facts
stop_conditions
```

`stop_conditions` 是历史兼容字段名，语义必须按 `completion / closure boundaries` 解释：它说明本轮交付的证据边界、不能强说的结论和下一步追查入口，不能被 runtime、评测器或 prompt 实现成永久硬停止或工具封锁。

### 5.4 受控开放查询规格

开放查询不是把完整工具菜单交给模型，也不是让未验证计算直接变成结论。它是 Codex 侦查思考进入确定性事实层之前的结构化请求。若开放查询需要超出语义工具的自定义计算，应升级为 Controlled Case Workbench 请求，并由来源信封、验证状态和 evidence boundary 决定能支撑什么结论。

| 字段 | 含义 | 门禁 |
| --- | --- | --- |
| `investigation_intent` | 用户真正要查的问题、怀疑点、报告目标或复核目标。 | 必须能映射到命令 lane 或 Lab 假设。 |
| `case_scope` | 当前案件、主体、账户、时间窗、方向、金额单位、成功/失败交易口径。 | scope 不明时先取 scope/card，不得直接出结论。 |
| `fact_need` | 需要哪些确定性事实：金额、笔数、账户、对手方、交易边、质量状态、claim 支持。 | fact_need 不能写成自然语言愿望，必须能落到事实层。 |
| `hypothesis` | Codex 提出的开放假设。 | 初始状态只能是 proposed / probing / lead，不得写成事实。 |
| `query_constraints` | 工具预算、深度、时间窗、金额 tolerance、是否允许图谱、是否允许报告。 | 超出当前预算时应先降级为 bounded answer、请求确认、切换 focused skill 或进入受控 workbench；不得把预算字段实现成跨轮硬停止。 |
| `evidence_requirement` | 需要 ledger、source refs、supported edge、coverage、claim review 的层级。 | 报告级输出必须强制 claim review。 |
| `output_shape` | Answer Card、Investigation Card、Flow Card、Continuation List、Report Card、QA Review。 | 输出形态决定 critique 和门禁。 |

Lab 运行优化规则：

- `plan_context` 可复用已经完成且 scope 匹配的计划结果，避免同一任务重复 plan。
- `skip_internal_plan` 只能在 plan context 匹配时生效；缺失或不匹配时，Lab 必须重新计划。
- `coverage_context` 必须区别于日期窗口 coverage，不能把计划复用中的 coverage 摘要当作报告级覆盖事实。
- 大账户集合必须使用 `account_key_count`、bounded previews 和 truncated flags 压缩；bounded preview 不能当完整账户集合。
- `mandatory_review_cards` 必须先处理，再进入报告写作或重大 claim 总结。
- Lab 完整卡片后，只有用户提出新假设、新对象、新时间窗、新路径或卡片允许 additional tools 时，才进入下一轮追查。
- `old_thread_patterns` 这类自由探索意图只表示侦查式不规律深挖 lane，不表示过程记录、历史数据或既定结论。

### 5.5 人工确认事实

人工确认事实是一类独立证据，不能和模型推断混同。人工确认必须记录确认来源、确认内容、确认时间、适用 scope、与原假设的关系和可撤销边界。人工确认可以把候选账户、重复族、混同路径或现金桥候选提升为更强事实，但仍要进入 Evidence Ledger 和 Claim Verifier。

## 6. 命令能力矩阵与受控意图路由

`/analytix` 是插件的稳定命名空间。命令体系不是工具菜单，而是把自然语言问题映射到稳定能力族，防止模型随意扫工具、错走报告路径或把专题线索写成事实。

### 6.1 稳定命名空间规则

- 用户只输入裸 `/analytix`、询问插件入口、能力总览或可用命令时，只解析当前案件和能力入口，不自动生成报告。
- 用户明确要求“全案分析/完整研判/按功能树跑完”时，进入 `full-case-analysis`，先跑全案分析树，不等同于报告门禁。
- 用户明确要求“生成报告/撰写报告/正式材料/报告初稿/附件”或 `/analytix report` 时，才进入报告工作流。
- 用户不写命令但资金语义清楚时，按意图进入对应命令族。
- 多个命令可能匹配且不确定点在数据、来源、范围时，先取案件范围、scope 或 casegraph。
- 多个命令可能匹配且不确定点在复杂度、执行 lane、预算时，先取分析计划。
- 简单账户、户名、金额、排行问题不能升级为全案报告。
- 开放式自由深挖先形成证据约束的假设卡，再按最高价值线索续查。
- 非资金任务保持 passive，不调用资金工具，不套资金研判结构。

### 6.2 命令主路径与门禁

| 命令 | 能力 lane | 使用场景 | 主路径 | 核心门禁 |
| --- | --- | --- | --- | --- |
| `/analytix ask` | frontdoor | 自然语言资金事实问答，低上下文入口。 | Codex 直接选用 scope/rank/profile/trace 等语义工具；模糊时用 `funds_investigate` navigator。 | 只输出 `key_facts` / `warnings` / `next_actions`；不暴露内部 id。 |
| `/analytix casegraph` | scope | 当前案件范围、质量门、Top 实体预览、重复/数据质量边界。 | `get_current_case` -> `get_case_scope_map` / `get_casegraph` | 返回紧凑 casegraph 语境；不扫描全部低层工具。 |
| `/analytix flowgraph` | path | 确定性资金流图、Mermaid 来源数据。 | trace seed -> `trace_fund_next_hop` / `trace_fund` / `build_fund_flow_graph`，必要时 `validate_continuation_list` | 只画 supported transaction edge；未匹配端点是补调对象。 |
| `/analytix visual` | delivery | 证据表格、Top20 图表、特征表、资金流向表、看板卡和附件目录。 | 已复核 facts -> visual/table inventory -> 必要时 `validate_continuation_list` / `validate_report_claims` | 表格/图表必须写 scope、unit、time window、metric、direction、evidence status；排行、矩阵、看板不是交易路径。 |
| `/analytix` | entry | 当前案件入口、能力总览、可用 lane 和下一步建议。 | `get_current_case` -> compact lane summary；必要时 `get_case_scope_map` | 不触发 full-case/report gate，不输出说明书长文。 |
| `/analytix report` | report | 生成或更新全案分析研判报告。 | 报告专项事实卡 -> `validate_report_claims` | 未通过 claim 复核时只输出阻断事实、待复核草稿和下一步验证动作。 |
| `/analytix scope` | scope | 导入、清洗、可用范围、报告级覆盖口径。 | current case -> scope map -> schema/source audit -> quality -> reconciliation -> coverage | import/source coverage、cleaned detail、analysis index、holder tree、report scope 必须分开；普通研判不使用 raw 表作事实来源。 |
| `/analytix map` | scope | 自由研判前的数据/范围图。 | `get_case_scope_map` 或 casegraph lane | 返回 nodes、edges、gate status、下一步 1-3 个能力动作；不做全工具扫。 |
| `/analytix audit` | qa | 导入清洗、非标表、空户名、换卡/补卡、同事实重复体检。 | `audit_case_data_quality` + source/scope facts | source、dedupe、holder、card replacement 有歧义时写“需复核”。 |
| `/analytix duplicates` | qa | 重复数据、同账号不同卡、同事实跨账户候选族。 | `resolve_duplicate_families` + coverage | 只报告候选重复族和候选影响；未确认同主体时不得扣减 totals。 |
| `/analytix compare-scopes` | qa | 行数、金额、户名树、报告去重口径复核。 | `compare_analysis_scopes` + reconciliation + coverage | 每个金额或计数必须说明来自哪个 scope。 |
| `/analytix rank` | ranking | 全案 / 户名 / 账户 / 对手方 Top 排名。 | `rank_accounts` / `rank_holders` / `rank_counterparties` | 必须写 metric、direction、scope、success_filter、coverage、金额单位。 |
| `/analytix account <account>` | account | 单账户统计、画像、特征、证据包。 | current case -> reconciliation -> resolve account -> coverage -> account profile -> evidence pack | 账户 totals 只能来自 aggregate/profile；bounded rows 只能作例证。 |
| `/analytix owner <name>` | holder | 某人/主体直接账户与候选关联账户。 | `analyze_holder_full` / holder scope facts | direct registered accounts 与 candidate-linked accounts 分离。 |
| `/analytix holder <name>` | holder | 户名/主体画像、账户集、交易统计、候选边界。 | `analyze_holder_full` + rank/trace as needed | holder card 完整时可回答；不得继续裸跑低层工具。 |
| `/analytix discovery` | hypothesis_lab | 开放式发现现金、理财、工程、涉诉、资产、三方支付等线索。 | current case -> optional owner scope -> discovery scan / hypothesis probe | broad discovery 只能输出 candidate lead，必须给事实字段和下一步动作。 |
| `/analytix lab` | hypothesis_lab | 自由深挖入口：假设队列、证据缺口、禁止事实化。 | `hypothesis_probe` + targeted rank/trace/quality tools | 先返回 Investigation Lab Card；假设必须标注状态。 |
| `/analytix destination` | subject_outflow_tracing | 某卡 / 某人资金去向、补调对象、一跳金额。 | `trace_subject_top_outflows` / `trace_fund_next_hop` | 一跳金额题不能扩成图谱发现；确定性去向、后续追查对象、unsupported flow 必须分开。 |
| `/analytix outflows` | subject_outflow_tracing | 主体 Top 出账、终点分类、穿透到无下游、补调清单。 | `trace_subject_top_outflows` -> `trace_fund_next_hop`，正式附件再 `validate_continuation_list` | unmatched endpoints 是补调目标，不是最终流向结论。 |
| `/analytix probe` | hypothesis_lab | Agent 自定义侦查假设校验。 | scope -> `hypothesis_probe` -> evidence pack | Codex 可以提假设，但必须由 MCP-backed evidence 支撑。 |
| `/analytix pattern` | pattern | 不规律性深挖：重复交易号、缺失对手、金额集中、人名/理财关键词链路。 | scope -> data quality / duplicate -> `hypothesis_probe` -> targeted trace/classify/QA | 单位、重复、缺失对手和关键词链路都不能事实化过度。 |
| `/analytix cash` | pattern | 现金存取、ATM/柜面、户名缺失、现金断点。 | scope -> cash breakpoint -> missing-counterparty classify -> evidence pack | 现金表述默认是线索；未验证桥接时间和身份时不能写成同一笔现金。 |
| `/analytix financial` | pattern | 理财、基金、证券、保险类资金线索。 | scope -> financial product flow -> rule / keyword evidence -> evidence pack | 产品关键词优先于现金/普通转账误判。 |
| `/analytix topic` | pattern | 工程、涉诉、资产消费等专题线索。 | scope -> topic detector / hypothesis probe -> evidence pack | 只能输出 topic lead card；未有补充材料不得证明交易目的。 |
| `/analytix cash-bridge` | pattern | 同日/次日取现-存现对应特征。 | scope -> cash breakpoints / hypothesis probe -> evidence pack | 只能写 cash bridge candidates；不能仅凭流水认定同一物理现金。 |
| `/analytix payment` | pattern | 支付宝、财付通、微信、银联、网联等支付通道。 | scope -> rule hits / keyword slices -> evidence pack | 只能按规则/关键词事实写通道线索；不得推断平台账户控制或权属。 |
| `/analytix crypto` | pattern | 虚拟资产购置或出入金线索。 | scope -> strict rule / keyword hits -> bounded facts -> evidence pack | 弱证据必须降级；没有强通道事实不得写虚拟资产结论。 |
| `/analytix path <seed>` | subject_outflow_tracing | 资金链路、下一跳、回流、闭环候选。 | current case -> resolve seed -> `trace_fund_next_hop` / `trace_fund` -> evidence pack | 只展示工具返回的确定性 in-case hops；不能靠推理补画缺失跳。 |
| `/analytix role` | ranking | 账户角色候选：归集、过账、沉淀、生活消费等。 | account stats / dashboard / risk scan -> evidence pack | 角色是资金流角色候选，不是法律身份或犯罪角色。 |
| `/analytix asset` | pattern | 购车、购房、理财、保险、借贷、物业停车等资产/消费线索。 | rule hits / asset detector -> evidence pack | 输出资产消费线索和补证建议；不得写最终资产归属。 |
| `/analytix control` | holder | 代持、实际控制、共用环境、候选控制线索。 | owner scope -> dashboard / risk scan / hypothesis probe -> evidence pack | 控制/代持只能写线索；需要身份、设备、授权、回单等外部证据闭合。 |
| `/analytix evidence` | qa | 补调账户、银行、支付机构、材料和附件清单。 | follow-up list / top outflows / risk scan -> evidence pack -> `validate_continuation_list` | 按金额、风险理由、缺失字段、预期证明力排序。 |
| `/analytix plan` | planning | 复杂度、执行 lane、工具预算、是否需要专项流程。 | current case -> `plan_case_analysis` | 只返回 lane plan 和 allowed tools；不执行全工具。 |
| `/analytix qa` | qa | 数字、口径、措辞、报告/附件缺口复核。 | compare scopes / coverage / `validate_report_claims` / `validate_continuation_list` | numbers、dates、units、fact origin、unsupported conclusions、coverage 任一缺口都阻断最终措辞。 |

### 6.3 命令能力族归档

所有 `/analytix` 命令必须归入稳定能力族。命令是用户入口，能力族是产品能力边界，Capability Registry 是机器事实源，MCP runtime 是确定性执行层。任何新增、合并或删除命令都不能只改展示文案，必须同步能力族、路由、answer contract、doctor 和 eval。

| Capability id | 能力族 | 覆盖命令 | 产品职责 | 质量/边界门禁 |
| --- | --- | --- | --- | --- |
| `frontdoor-qa` | 普通事实问答前门。 | `/analytix ask` | 把自然语言资金问题收束为一次低上下文事实卡。 | 不升级全案报告；不暴露内部 id；事实不足时给边界和下一步。 |
| `scope-casegraph` | 范围、质量门和案件图谱入口。 | `/analytix casegraph`、`/analytix scope`、`/analytix map` | 在自由探索前建立可见数据、清洗、覆盖、质量和 Top 实体的低上下文图谱。 | raw 数据不进上下文；范围不明不写全量。 |
| `fundgraph-path` | 确定性资金流图。 | `/analytix flowgraph` | 将 supported transaction edge 编译为 Flow Card、Mermaid source table 和 fundgraph。 | 聚合排行不能画成交易边；无 supported edge 不画箭头。 |
| `report-claim-gate` | 全案报告和计划门禁。 | `/analytix`、`/analytix report`、`/analytix plan` | 生成 lane plan、全案报告事实卡、报告草稿和 claim review。 | 报告、重大 claim、Mermaid 和附件必须进入复核。 |
| `qa-source-review` | 数据质量、重复、口径和补证复核。 | `/analytix audit`、`/analytix duplicates`、`/analytix compare-scopes`、`/analytix evidence`、`/analytix qa` | 检查导入清洗、source scope、重复候选、单位、字段缺口和续调清单。 | 不用样本扣减 totals；缺证只能列补证动作。 |
| `ranking-subject` | Top 排名和账户角色候选。 | `/analytix rank`、`/analytix role` | 提供账户、户名、对手方、角色候选的确定性排名。 | metric、direction、scope、coverage、单位缺一不可。 |
| `holder-account-scope` | 主体、账户、户名和控制线索画像。 | `/analytix account <account>`、`/analytix owner <name>`、`/analytix holder <name>`、`/analytix control` | 建立直接账户、候选关联账户、账户统计、控制/代持线索边界。 | candidate-linked accounts 不能写成 direct registered accounts。 |
| `investigation-lab` | 开放式假设实验室。 | `/analytix discovery`、`/analytix lab`、`/analytix probe` | 将自由侦查问题编译为假设队列、证据状态、缺口和下一步追查。 | 假设必须分级；线索不能事实化；每轮有预算和收口边界。 |
| `subject-outflow-tracing` | 主体资金去向和路径追查。 | `/analytix destination`、`/analytix outflows`、`/analytix path <seed>` | 沿主体、账户、金额、时间和下一跳追踪可见资金去向。 | unmatched endpoint 是补调目标，不是最终去向。 |
| `pattern-and-asset-leads` | 专题模式、现金、支付、资产和金融产品线索。 | `/analytix pattern`、`/analytix cash`、`/analytix financial`、`/analytix topic`、`/analytix cash-bridge`、`/analytix payment`、`/analytix crypto`、`/analytix asset` | 发现不规律交易、现金断点、金融产品、支付通道、资产消费和专题线索。 | 只能输出 candidate lead 和补证动作，不能证明交易目的或资产归属。 |

命令能力族的校验规则：

- 每个命令必须属于且只属于一个 primary capability。
- 一个能力族可以服务多个命令，但必须有统一输出准则和统一门禁。
- 命令 metadata、Capability Registry、tool schema、doctor、eval 中的 lane 必须一致。
- 同一个用户问题命中多个能力族时，先用 Task Profile 决定主 lane，再把其他 lane 写成补充追查或 QA gate。
- 问题已经形成完整 Answer Card 时，不得继续转入无预算 specialist sweep。
- 任何命令都不能绕过 Evidence Ledger、Context Compiler 和对应输出规则。

### 6.4 路由 fallback

- 用户没有写精确命令，但中文语义清楚时，按意图映射到命令族。
- 不确定点在数据、来源、范围时，先走 `/analytix casegraph` / `/analytix scope` / `/analytix map`。
- 不确定点在复杂度、执行 lane、预算时，先走 `/analytix plan`。
- 简单账户、户名、金额、排行题足够回答时，不能升级成全案报告。
- 用户说“全面梳理、资金追踪、资金关联通道、穿透、找到路径、围绕多名主体/关联人/关联公司追查”等侦查式问题，默认进入 `investigation-lab` 或 `subject-outflow-tracing`，不因文字出现“全面”就进入报告门禁。
- 只有用户明确要求“生成/撰写/输出全案报告、正式材料、研判报告、报告目录、报告初稿、附件”或显式 `/analytix report` 时，才进入报告工作流。
- 多主体但未给 `holder_name` 时，Task Profile 必须从用户问题提取 `focus_keywords`、主体候选和关联公司候选，不能填入固定示例主体、固定时间窗或固定交易号。
- 路由输出必须保留用户原始侦查目标；Answer Card 标题、侦查意图、假设队列和下一步不得替换成测试主体或历史 golden 主体。
- 开放式深挖先走 `/analytix lab` 的假设卡，再按 highest-value lead 续查。
- 专题线索题先输出 candidate lead 和证据缺口，不直接进入法律结论。
- 非资金任务保持 passive，不调用资金工具。
- 命令不在 `command-metadata.json` 时，不得临场发明工具链；回到 `/analytix ask`、`/analytix plan` 或请求用户缩小范围。

### 6.5 受控意图路由契约

```text
Command Router
-> Task Profile
-> Capability Set
-> Intent AST
-> Plan DAG
-> Deterministic Tool Runtime
```

- `tools/list` 能力变化既是 MCP 协议事实，也决定 Codex 可见的认知工具箱，必须纳入生产质量核准。
- 生产默认工具面保持语义充足、低层收缩，由 Codex 根据用户问题和 Task Profile 选择 Capability Set。
- 完整低层工具发现只在开发、诊断、显式授权或专项 eval 中使用。
- 新能力先进入 Capability Registry，再生成 command metadata、tool schema、doctor、eval 和 answer contract。
- 任何路由结果都必须有预算、收口边界、证据要求和被动非资金测试。
- 路由不是“动态路由越多越好”。插件采用受控意图路由：自然语言可以自由表达，但落点必须是稳定命令族、稳定 capability、稳定 answer contract 和稳定门禁。
- 如果路由误把侦查式追踪问题送入报告写作门禁，或误把报告请求送入普通事实问答，均属于 route violation，必须进入 frontdoor smoke、真机功能验收和 release-quality auto-fail。

## 7. 工程架构蓝图

### 7.1 工程分层

```text
Plugin Package Boundary
-> Skill Boundary
-> Capability Registry
-> Command Router
-> Task Profile
-> Capability Set
-> Intent AST
-> Plan DAG
-> Deterministic Tool Runtime
-> Evidence Ledger
-> Context Compiler
-> Output Rule Registry
-> Anti-pattern Detector
-> Critique Pass
-> Casegraph / Fundgraph
-> Investigation Lab
-> Claim Verifier
-> Answer / Flow / Report Card
```

### 7.2 Capability Registry 机器契约

Capability Registry 是插件能力的上游事实源。它不是说明文档，也不是命令菜单，而是驱动命令 metadata、工具预算、doctor、功能测试、质量维度和 answer contract 的机器契约。

Registry 必备字段语义：

| 字段 | 语义 | 漂移风险 |
| --- | --- | --- |
| `schema_version` | registry schema 的机器校验依据。 | schema 漂移会导致 doctor、eval 和工具描述不一致。 |
| `plugin` | 插件身份、能力包名称和边界。 | 插件身份与 manifest / MCP server 不一致。 |
| `tool_discovery_contract` | 默认 source-backed 能力层、完整低层工具发现条件、低层调试工具面。 | 工具面过度收缩导致 Codex 只能走机械前门，或低层工具全暴露导致菜单扩散。 |
| `eval_coverage_contract` | 每个能力族必须覆盖的任务、风险和 auto-fail。 | 质量证据只覆盖 happy path。 |
| `capabilities` | 能力 id、lane、commands、tools、gates、budget、output、eval。 | 命令、工具、报告门禁和测试分裂。 |

每个 capability 必须定义：

- `id`：稳定能力标识，不随文案改动变化。
- `lane`：frontdoor / scope / path / report / planning / qa / ranking / holder / hypothesis_lab / subject_outflow_tracing / pattern。
- `commands`：绑定的 `/analytix` 命令入口。
- `primary_tools`：能力常用语义事实工具，供 Codex 按问题自主选择，不等同于固定链路。
- `allowed_specialists`：重型、低层、写入、导出、debug 或正式报告工具，只有在 plan、recommended next action 或显式任务需要时才允许。
- `required_gates`：scope、coverage、ledger、claim review、continuation validation、anti-pattern、passive 等门禁。
- `budget`：最大重工具数量、是否允许报告链路、是否允许多轮追查。
- `answer_contract`：Answer Card、Investigation Card、Flow Card、Report Card 或 QA Review 的字段要求。
- `eval_tasks`：golden、near-miss、passive、adversarial、report-review、cost regression 的覆盖。
- `drift_checks`：registry、command metadata、tool schema、doctor、eval、reference 文档一致性。

Registry 派生关系：

```text
Capability Registry
-> command-metadata.json
-> tool discovery policy
-> tool schemas / input schemas
-> frontdoor routing
-> output rule registry
-> doctor contract
-> eval coverage
-> quality dimensions
```

任何能力扩展必须先改变 registry，再由 registry 驱动程序和测试。不能先加工具、后补文档；不能只加命令、不加门禁；不能只补 prompt、不补 evidence 和 eval。

#### Registry 派生契约与运行变量门禁

Registry 中的工具发现契约必须明确区分默认可见、前门报告链路、优先工具、完整发现和调试路径。

| 字段 | 契约含义 | 蓝本要求 |
| --- | --- | --- |
| `priority_tools` | 插件事实层的高价值语义工具集合。 | 必须成为 Codex 默认认知工具箱的候选来源，不得只藏在 Plan DAG 后面。 |
| `default_visible_tools` | 普通 Agent 使用时可见的语义事实工具箱。 | 应覆盖 scope、rank、profile、trace、casegraph、hypothesis、quality、validation；不得退化为单一前门。 |
| `frontdoor_report_only_tools` | 正式报告任务允许的前门与 claim gate 工具组合。 | 报告不靠全工具扫；报告门禁不得扩散到普通研判、追踪或改口径重算。 |
| `full_discovery_env_vars` | 开发、诊断或专项 eval 才能启用完整低层工具发现的变量名。 | 生产普通任务不能依赖 raw/debug/full discovery。 |
| `max_default_visible_tools` | 默认可见工具数量上限。 | doctor 阻断低层工具扩散，但不能以数量上限压掉必要语义事实工具。 |
| `conditional_tools` | 满足 gate、next action 或显式任务时才允许的重型/低层工具。 | 高价值语义事实工具不应被误放入条件隐藏区。 |
| `doctor_checks` | 结构、字段、边界和质量证据检查。 | 每个 capability 至少能被 doctor 或 eval 覆盖。 |
| `quality_dimensions` | 事实准确、边界、报告、成本等质量维度。 | 功能测试和 golden QA 必须能落到这些维度。 |

运行变量门禁：

| 变量 / 参数 | 允许用途 | 禁止用途 |
| --- | --- | --- |
| `ANALYTIX_FUNDS_DISCOVERY_MODE` | 开发诊断或专项 eval 中打开完整低层工具发现。 | 普通用户任务依赖全工具列表。 |
| `ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS` | 与完整发现等价的受控诊断开关。 | 绕过低层工具收缩，把 raw/debug/heavy 工具暴露给普通任务。 |
| `include_debug` | 受控环境下请求完整 structuredContent 或 debug payload。 | 在普通输出中暴露 raw payload。 |
| `ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD` | 与 `include_debug` 配合，显式允许 debug payload。 | 单独作为用户可见证据来源。 |
| `ANALYTIX_API_BASE_URL` | 指向 Analytix-owned backend。 | 写死到发布包、泄露环境、连接非 Analytix 数据源。 |

调试与完整发现的结果只能进入本地 artifact、内部 trace 或 `_meta`，不得进入用户正文，也不得成为生产路径的必要条件。

### 7.3 Natural-language Navigator 与 Tool Choice Contract

`funds_investigate` 的职责不是替所有任务生成固定链路，也不是把 Codex 变成只读 Answer Card 的事实查询器。它是自然语言 shortcut / navigator：当用户问题模糊、跨能力或需要快速落到案件事实时，帮助 Codex 识别任务画像、建议语义事实工具、给出可直接回答的最小事实和下一步缺口。普通研判的工具选择权必须保留给 Codex；正式报告才进入 report lane 与 claim gate。

Navigator 必须输出：

| 字段 | 含义 | 使用规则 |
| --- | --- | --- |
| `task_profile` | 用户问题对应的任务画像。 | 只作为路由建议；不能替代 Codex 对新对象、新口径、新时间窗和新假设的判断。 |
| `suggested_tools` | 建议下一步使用的语义事实工具。 | 建议而非门禁；Codex 可以直接选择 rank/profile/trace/hypothesis/quality 工具。 |
| `direct_answer_facts` | 当前已足以回答的事实。 | 必须有 scope、单位、来源边界和 forbidden-as-fact。 |
| `required_facts_present` | 金额、笔数、账户、主体、对手、scope 等必要事实是否齐备。 | false 时只能输出边界和补证动作。 |
| `unsupported_flows_present` | 是否存在无支持资金边或无法证明链路。 | true 时禁止 Mermaid 确定箭头和最终去向表达。 |
| `hypothesis_queue` | 候选假设、反证方向、证据缺口。 | 输出为 lead / gap / next tool，不得升级为事实。 |
| `recommended_next_action` | 最高价值下一步。 | 必须是语义事实工具、命令族、补证动作或人工确认，不是笼统“继续分析”。 |
| `internal_stop_state` | 本轮链路是否需要停止。 | `answer_card_complete`、`max_additional_tools` 等只作为内部预算状态，不得作为用户可见结论，也不得压制下一轮新任务。 |
| `key_facts` | 可直接向用户呈现的事实。 | 必须有 scope、单位和边界。 |
| `warnings` | coverage、重复、候选、缺失对手、单位、可见数据停止点。 | 不能被输出编译器删除。 |
| `next_actions` | 可交付的续查动作。 | 必须说明对象、字段、预期证明力。 |
| `forbidden_as_facts` | 禁止写成事实的内容。 | 输出前 Critique Pass 必须检查。 |
| `context_compiler` | 事实卡压缩策略、raw payload 边界和 artifact 关系。 | raw rows、大 JSON、内部 id 不进入用户正文。 |

Navigator 必须覆盖四类结局：

- 完整回答：事实齐备、scope 清楚、无需额外工具。
- 边界回答：事实不足但可以说明已知、未知和下一步。
- 深挖回答：形成假设队列，并给出预算内最高价值追查。
- 阻断回答：数据、scope、工具或证据不足以支撑强结论。

禁止的 navigator 行为：

- 不得把 Top、画像、穿透、开放深挖全部强行转入 `funds_investigate` 固定链。
- 不得在普通研判中输出 `answer_card_complete=true`、`max_additional_tools=0`、`write_blocked`、`report gate`、`diagnostic label` 等内部控制语。
- 不得因本轮 answer card complete 而拒绝用户“继续追一层”“换口径重算”“查另一个主体”“做否定性搜索”。
- 不得把报告 claim gate 扩散成普通资金追踪、排名或主体画像的前置门禁。

### 7.4 MCP 程序载体地图

MCP 不是工具列表，而是确定性事实层、输出编译层、证据账本和质量门禁的程序载体。每个模块必须有清晰职责、输入输出和风险边界。

| 程序载体 | 职责 | 对应能力 |
| --- | --- | --- |
| `server.mjs`、`jsonrpc-stdio-runtime.mjs`、`mcp-request-handler-runtime.mjs`、`mcp-tool-result-runtime.mjs` | MCP server 启动、JSON-RPC、请求处理、工具结果封装。 | 插件运行入口、协议稳定性、错误状态。 |
| `backend-api-client.mjs`、`runtime-normalizers.mjs`、`safe-skill-runtime.mjs` | analytix data/runtime backend 访问、返回结构归一化、skill 安全加载。 | 事实访问边界、runtime 安全。 |
| `tool-schemas.mjs`、`tool-input-schemas.mjs`、`tool-discovery-policy.mjs`、`tool-runtime-routing.mjs`、`tool-call-runtime.mjs` | 工具 schema、输入校验、默认 source-backed 能力层、低层工具路由和调用执行。 | 语义充足、低层受控、工具预算。 |
| `capability-registry-runtime.mjs` | 读取 registry、校验 capability、提供机器能力事实。 | 能力事实源、drift check。 |
| `frontdoor-runtime.mjs`、`frontdoor-routing.mjs`、`frontdoor-answer-contract.mjs`、`frontdoor-fact-summaries.mjs` | 自然语言 navigator、任务画像、answer contract、事实摘要。 | 普通问答 shortcut、报告入口、低上下文事实卡，不垄断工具选择。 |
| `intent-plan-protocol.mjs`、`answer-card-protocol.mjs`、`card-renderer.mjs` | Intent AST、Plan DAG、Answer Card schema 和卡片渲染。 | 侦查意图驱动、计划门禁、输出一致性。 |
| `case-pipeline-runtime.mjs`、`case-scope-map-runtime.mjs`、`stats-query-runtime.mjs` | 数据管线、scope map、统计查询和 bounded rows。 | source scope、coverage、rank、账户画像。 |
| `casegraph-protocol.mjs`、`casegraph-runtime.mjs` | 案件事实图谱协议和运行时。 | casegraph、scope、quality gate、claim support。 |
| `fundgraph-builder.mjs`、`fund-flow-graph-runtime.mjs` | supported transaction edge 构图和资金流运行时。 | fundgraph、Flow Card、Mermaid source。 |
| `investigation-lab-protocol.mjs`、`investigation-lab-diagnostic-runtime.mjs` | 假设实验室协议、mandatory review cards、开放式深挖诊断。 | hypothesis lab、多轮追查、假设分级。 |
| `destination-diagnostic-runtime.mjs`、`top-rankings-diagnostic-runtime.mjs`、`claim-review-diagnostic-runtime.mjs`、`diagnostic-fact-helpers.mjs` | 去向、排行、claim review 和诊断事实帮助器。 | 资金去向、Top、报告复核、诊断卡。 |
| `evidence-ledger.mjs`、`context-compiler.mjs`、`mcp-artifact-store.mjs`、`mcp-output-policy.mjs` | 证据账本、上下文编译、artifact 隔离、输出策略。 | 可追溯事实、低上下文、raw output 隔离。 |
| `claim-verifier-protocol.mjs` | 报告 claim、资金边、禁用表述和证据锚点复核。 | 报告门禁、adversarial review。 |
| `agent-payload-compiler.mjs`、`agent-output-compiler.mjs`、`agent-context-hygiene.mjs` | Agent 可读 payload、用户可见输出、上下文卫生。 | 不暴露内部 id、不塞 raw payload、输出口吻。 |
| `progressive-resources.mjs` | MCP resources 渐进披露，提供 skill、reference 和 registry 资源。 | 短 skill、按需加载、reference 一致性。 |

程序载体地图必须和 Reference 映射保持一致。新增模块如果不能映射到能力族、证据面、输出卡片或质量门禁，就不应进入插件主线。

### 7.5 工具发现、可用能力与降级协议

默认工具面保持“语义充足、低层收缩”：

- 当前案件与范围：`get_current_case`、`get_case_scope_map`、`get_casegraph`、`get_scope_coverage`。
- 排名与画像：`rank_accounts`、`rank_holders`、`rank_counterparties`、`analyze_account_full`、`analyze_holder_full`。
- 资金链路：`trace_subject_top_outflows`、`trace_fund_next_hop`、`trace_fund`、`build_fund_flow_graph`。
- 开放假设与质量：`hypothesis_probe`、`audit_case_data_quality`、`resolve_duplicate_families`。
- 交付复核：`validate_continuation_list`、`validate_report_claims`。
- 自然语言导航：`funds_investigate`，只用于 shortcut / route hint / lightweight navigator。

完整低层工具发现只允许出现在开发诊断、显式授权、专项 eval 或 doctor 场景。普通 Agent 使用不通过 `tools/list` 暴露 raw rows、debug、写入、导出和全案报告重链路工具；高价值语义工具可以由 Codex 根据用户问题直接选择，复杂报告、批量导出或重型链路再由 Plan DAG、recommended next action 和工具预算决定。

缺失或受限能力处理规则：

| 缺失能力 | 输出策略 | 禁止行为 |
| --- | --- | --- |
| 账户角色分类不足。 | 用账户统计、dashboard、risk scan 输出 candidate role。 | 写成确认角色或法律身份。 |
| 现金桥检测不足。 | 输出 cash bridge candidates、时间/金额/主体特征和补证动作。 | 认定同一物理现金或最终去向。 |
| 混同资金分配器不足。 | 写明不可精确归因，提供可见路径和缺口。 | 按比例或主观判断分配涉案金额。 |
| 新表头、非标准表或 source drift。 | 先走 scope map、schema audit、unindexed source audit。 | 把暂不可用当作没有数据。 |
| 重复、换卡、同事实候选不闭合。 | 输出候选族和影响范围。 | 全案盲扣 totals。 |
| 报告 validator 不可用。 | 阻断正式报告措辞，输出复核限制和人工校验清单。 | 继续交付无门禁正式报告。 |
| 专题探针不足。 | 降级为线索卡和补充材料需求。 | 证明交易目的、资产归属或控制关系。 |

工具失败处理规则：

- 工具失败不能当作零值事实。
- 后端 API 暂不可用不能写成无交易、无账户或无风险；若案件项目 DuckDB 可读，应降级为 warning 并继续走本地只读事实层。
- schema 暂不可用只能写为 retry boundary。
- 任何 fallback 不得绕过当前案件项目绑定、只读 DuckDB guardrail、MCP/Workbench 审计和证据边界。
- 用户需要继续时，下一步必须是受控命令、补证动作、Case Workbench 分析或人工确认，而不是无来源信封的结果搬运。

### 7.6 模块职责

| 模块 | 输入 | 输出 | 失败模式 | 质量证据 |
| --- | --- | --- | --- | --- |
| Capability Registry | capability、commands、tools、budget、gates、eval tasks。 | 统一能力事实源。 | command metadata、tool schema、doctor、eval 漂移。 | schema 校验、command coverage、tool budget 校验。 |
| Command Router | `/analytix` 命令、中文自然语言问题、任务画像。 | 命令族、lane、primary tools、required gates、stop conditions。 | 命令变工具菜单；普通问答升级全案报告。 | command metadata 与 router 一致；passive 任务不误触发。 |
| Task Profile | 用户问题、案件上下文、显式命令。 | fact / rank / holder / destination / lab / qa / report / passive。 | 任务画像误判导致工具扩散。 | golden tasks、near-miss passive tasks。 |
| Intent AST | normalized intent、主体、时间窗、account keys、report flag。 | 结构化意图。 | 把模糊问题硬转为强结论。 | intent fixtures、schema 校验。 |
| Plan DAG | Intent AST、tool budget、claim review flag。 | 工具节点、门禁、收口边界。 | 重复 discovery、报告绕过复核。 | budget eval、重复调用检查。 |
| Deterministic Tool Runtime | case id、工具参数、backend response。 | 事实、聚合、图谱边、质量门。 | raw rows 进上下文；bounded rows 算全案。 | smoke、health、raw payload 策略检查。 |
| Evidence Ledger | facts、edges、claims、source refs。 | 可追溯证据账本。 | 金额、链路、claim 无证据。 | ledger coverage、unanchored sample rejection。 |
| Context Compiler | raw structured result、card、warnings。 | 研判事实卡。 | 大 JSON 进模型；内部 id 外泄。 | compact text limit、opaque ref scan。 |
| Output Rule Registry | ask / qa / rank / lab / flowgraph / report / evidence / review。 | 输出准则、术语状态、降级规则。 | 规则只存在文档，不影响输出。 | command metadata、anti-patterns、eval 维度覆盖。 |
| Anti-pattern Detector | 输出文本、事实卡、报告草稿、Mermaid。 | P0/P1/P2 风险。 | 只做主观审查。 | deterministic patterns、doctor 和 eval gates。 |
| Critique Pass | 用户可见输出。 | 事实边界、假设分级、口吻和工程黑话审查。 | 只在报告末端复核。 | 普通问答、排行、图谱、lab、report 均有 critique 覆盖。 |
| Casegraph | case scope、rank facts、quality gates、claim facts。 | 主体/账户/对手/交易/质量/证据图。 | 图谱漫游、无证据边。 | minimum roadmap、claim support、edge guard。 |
| Fundgraph | supported transaction seed rows。 | 资金节点和 supported edges。 | 聚合排行画成交易边。 | supported edge contract、Mermaid guard。 |
| Investigation Lab | 侦查目标、主体、关键词、scope facts。 | 假设队列、证据状态、缺口、下一步。 | 假设升级为事实。 | lab eval、mandatory review cards。 |
| Claim Verifier | 报告文本、claims、facts、flow graph。 | verified / corrected / unsupported / forbidden / next actions。 | 报告 claim 未绑定事实。 | synthetic text risk、report claim eval。 |

### 7.7 冲突消解顺序

```text
案件隔离与权限边界
> 确定性事实与证据门禁
> 用户问题和侦查意图
> 自主假设与多轮追查
> 低工具 / 低上下文成本
> 表达风格和交付格式
```

## 8. 证据链、图谱与上下文协议

### 8.1 领域对象模型

| 对象 | 定义 | 边界 |
| --- | --- | --- |
| Case | 选定案件和可见数据范围。 | 不能跨案取事实。 |
| Case Journal | 本案内多轮研判状态。 | 不形成跨案件长期事实记忆。 |
| Source Scope | 导入、清洗、分析索引、聚合之间的映射。 | 不同层级数字不能混用。 |
| Subject / Holder | 户名、人员、单位研判对象。 | 候选关联不能写成名下账户。 |
| Account | 银行账户或支付账户。 | 换卡、补卡、同事实重复需复核。 |
| Counterparty | 交易对手方或对手账户。 | 缺失对手方不能补想象主体。 |
| Transaction Fact | 单笔或可追溯交易事实。 | 只有逐笔事实支持交易边。 |
| Aggregate Fact | 聚合统计事实。 | Top / 最大 / 总额必须说明 metric 和 coverage。 |
| Fund Edge | 资金流图边。 | 只有 supported edge 能进入 Mermaid / fundgraph。 |
| Hypothesis | 开放式侦查假设。 | 必须标注状态。 |
| Evidence Pack | 事实、边、source refs、warnings、coverage。 | 不等同最终报告结论。 |
| Claim | 用户可见或报告文本中的事实主张。 | unsupported / forbidden 必须删除、纠正或降级。 |

### 8.2 Evidence Ledger

Evidence Ledger 是插件可复核性的根。它必须覆盖所有可被用户引用、写入报告、进入图谱或生成附件的事实面。

必备证据面：

- amount。
- count。
- account。
- holder。
- counterparty。
- flow_or_transaction_edge。
- report_claim。
- source_refs。

账本条目必须保存：

| 字段 | 含义 |
| --- | --- |
| `ledger_id` | 内部账本标识，仅用于回溯，不直接暴露给用户。 |
| `fact_type` | amount / count / account / holder / counterparty / edge / claim / attachment_row。 |
| `support_status` | supported / corrected / lead / missing / downgraded / forbidden / unsupported。 |
| `scope` | case、source、holder/account、time window、direction、dedupe policy、success filter。 |
| `value` | 金额、笔数、账户、户名、对手方、交易边或 claim 文本。 |
| `source_refs` | backend / MCP / artifact / source hash / detail refs。 |
| `warnings` | coverage、重复、候选归属、单位、缺失字段、可见数据离开点。 |
| `review_action` | accept / correct / downgrade / block / ask_human / supplement。 |

用户可见输出只呈现事实字段、口径和可解释证据来源，不暴露不透明内部 id、artifact id、audit ref、debug ref 或 token。

#### 内部证据字段与用户可见边界

内部证据字段用于程序追溯、doctor、eval、claim review 和 trace replay，不等于用户正文引用格式。蓝本要求内部可追溯、外部可解释。

| 字段 | 内部含义 | 用户可见边界 |
| --- | --- | --- |
| `surface_signals` | 汇总一组事实是否覆盖金额、笔数、账户、户名、对手方、资金边和 claim。 | 不直接展示字段名，只展示被支持的事实和口径。 |
| `coverage_summary` | 证据账本覆盖状态和缺失锚点统计。 | 展示 coverage 风险和缺口，不展示内部扫描索引。 |
| `claim_support_index` | 把 claim review 结果挂回事实族、ledger 和复核动作。 | 展示 verified / corrected / unsupported 等状态，不展示 opaque index。 |
| `risk_marker` | 标记 claim 风险类型，如金额无事实锚点、unsupported flow、法律越界等。 | 转写为可读风险说明和纠正动作。 |
| `fact_refs` | 内部事实锚点。 | 用户正文只写事实字段、金额、时间、账户、对手方和 scope。 |
| `source_refs` | 来源、工具、artifact 或 backend 证据引用。 | 可以概括为“来自确定性事实层/分析索引/报告复核”，不暴露内部 id。 |
| `claim_review_risk_markers` | 报告复核的风险枚举集合。 | 输出为阻断点、降级理由和下一步验证动作。 |

内部字段使用规则：

- 内部字段必须能被 doctor、eval 或 claim review 检查。
- 内部字段不能以 `q_xxx`、`audit_ref`、`detail_ref`、`artifact_id`、`claim_id` 等形式出现在用户正文。
- 用户需要证据时，提供交易时间、金额、账户、对手方、scope、单位、字段来源和补证动作，而不是 opaque id。
- 证据字段缺失时，对应 claim 降级为 lead / missing / unsupported / needs_review。
- 内部字段可以支撑 trace replay，但不能成为跨案记忆。

### 8.3 Context Compiler

Context Compiler 的目标是让模型看到“足够研判”的压缩事实，而不是看到原始数据库结果。它对应的产品能力不是“摘要”，而是证据感知的最小充分上下文。

研判事实卡必须包含：

- 当前任务意图。
- 可直接写入的事实。
- 必须说明的边界。
- 禁止写成事实的内容。
- 下一步追查问题。
- 收口边界。
- 报告级是否需要 claim 复核。

raw result 策略：

- 后端 payload 留在本地 artifact。
- MCP debug payload 默认不进入模型正文。
- 大结构数据进入 `_meta` 或内部 trace。
- 证据账本可以进入 `_meta.analytix_evidence_ledger` 供程序回溯，但用户可见文本只呈现可解释事实、口径和边界。
- 用户可见文本只保留压缩事实、边界和补证动作。
- 压缩不能删除 scope、反证、候选、边界和可钻取方向。

### 8.4 Casegraph 与 Fundgraph

casegraph 是案件事实图谱，fundgraph 是资金交易边图谱。二者不能混同。

casegraph 节点：

- 主体节点。
- 账户节点。
- 户名节点。
- 对手方节点。
- 同事实族。
- 规则命中。
- 理财/资产端/现金断点。
- 缺失回单。
- evidence pack。
- claim support。

casegraph 边：

- 数据层边：import -> cleaning -> analysis index。
- 范围边：案件 -> 主体 -> 账户 -> 对手方。
- 质量边：duplicate、coverage、source audit、claim gate。
- 研判边：线索、缺口、补证动作。

fundgraph 边：

- 只能来自逐笔交易事实。
- 必须有时间、金额、方向、from、to。
- 必须有 supported 状态。
- 只能表达返回的交易边，不能续画未返回下游。

### 8.5 Claim Verifier

Claim Verifier 是报告级硬门禁。它负责在正式报告文本进入交付前，检查 claim 是否有事实锚点、是否存在禁用表述、是否误画链路。

检查项：

- 金额 claim 是否绑定 facts。
- Mermaid / flowchart / 箭头是否有 supported transaction edge。
- 候选账户是否被写成名下或归属事实。
- 现金、取现、理财、资产端是否被写成最终去向。
- 缺失对手方、未知对手方、空户名是否被写成已闭环。
- 涉黑、违法所得、实际控制、代持、最终资金归属是否越权。
- 每条敏感 claim 是否有 source refs / evidence refs / fact refs。

输出状态：

- verified claims。
- corrected claims。
- unsupported claims。
- unsupported flows。
- missing source boundaries。
- forbidden phrasings。
- next review actions。

### 8.6 Diagnostic Card Family

诊断卡族用于把 specialist 诊断结果转成可复核、可继续追查、可被报告复用的事实卡。诊断卡不是调试日志，也不是用户可见 raw output。

| 诊断卡 | 输入 | 输出 | 证据要求 |
| --- | --- | --- | --- |
| Destination Diagnostic Card | 主体、账户、方向、时间窗、金额统计范围。 | 一跳去向、终点分类、无下游端点、补调对象、unsupported flows。 | 每个去向必须有 amount、count、counterparty、scope、visible-data cut-off。 |
| Top Rankings Diagnostic Card | metric、direction、scope、success filter。 | Top 账户、Top 户名、Top 对手方、角色候选。 | 排名必须有 metric、coverage、单位、排序依据和 tie/boundary。 |
| Investigation Lab Diagnostic Card | 侦查意图、主体、关键词、scope facts、计划预算。 | 假设队列、mandatory review cards、证据缺口、下一步追查、收口边界。 | 每张卡必须有 priority、must_cite、evidence_statement、claim_guard。 |
| Claim Review Diagnostic Card | 报告文本、claims、facts、Mermaid、附件表。 | verified、corrected、unsupported、forbidden、write_blocked、next actions。 | 敏感 claim 必须绑定 fact refs；unsupported 必须阻断或降级。 |
| Data Quality Diagnostic Card | source、cleaning、coverage、duplicates、field quality。 | source audit、duplicate candidates、字段缺失、口径冲突。 | 不能把 audit 暂不可用写成无风险。 |
| Continuation Diagnostic Card | 补调账户、机构、材料、字段、证明目标。 | 优先级清单、缺失字段、预期证明力、重复风险。 | 必须通过续调清单校验，不能输出空字段和重复目标。 |

诊断卡编译规则：

- 诊断卡进入 Evidence Ledger；未进 ledger 的诊断不得支撑报告 claim。
- 诊断卡必须进入 Context Compiler 压缩；不得把完整诊断 payload 暴露给模型正文。
- 诊断卡可以支撑下一轮 Plan DAG，但不能自动触发无限追查。
- 诊断卡的 `warnings`、`claim_guard`、`forbidden_as_facts` 不能被报告编译器删除。
- 多张诊断卡冲突时，先以 scope、coverage、source audit、supported edge 和人工确认为准。

### 8.7 渐进披露、Resources 与 Artifact 协议

插件采用短 skill + references + scripts + assets 的渐进披露结构。模型默认只看到 skill 摘要和少量工具入口，需要专项知识时才加载对应 reference 或 MCP resource。

Resources 暴露规则：

- 可以暴露 root skill、command metadata、command router、tool availability、runtime boundary、Hub lifecycle、anti-patterns、plugin benchmark、casegraph roadmap、capability registry 等生产指导资源。
- golden、oracle、评测 rubric、真实评测答案和内部 debug 文件不作为 production resource 暴露。
- resource 内容只能指导路由、边界和输出门禁，不能把 eval 答案、临时过程或外部调研流水写入模型默认上下文。
- references 根目录与 skill 内嵌 references 必须保持一致。

Artifact 协议：

- raw rows、raw graph、长 JSON、tool debug、trace、ledger detail 留在 artifact / `_meta` / 本地 trace。
- 用户可见输出引用事实字段、scope、单位、边界和补证动作，不引用 opaque artifact id。
- artifact 只能在同一案件、同一任务或同一复核链路中被复用。
- artifact 过期、scope 变化、清洗状态变化、用户口径变化时，对应事实卡必须 stale。
- artifact 不得跨案变成长期记忆，也不得写入系统 Codex 状态。

## 9. 报告、附件与交付生命周期

### 9.1 交付物模型

| 交付物 | 使用场景 | 校验 |
| --- | --- | --- |
| Answer Card | 普通金额、Top、主体画像、去向问答。 | 金额、笔数、scope、warnings、stop condition。 |
| Investigation Card | 开放式假设、专题线索、异常发现。 | 假设状态、证据缺口、禁止事实化、下一步追查。 |
| Flow Card | 一跳去向、下一跳、多跳路径。 | supported edge、Mermaid 事实表、停止点。 |
| Evidence Pack | 报告、图谱、续调清单、QA 的证据底座。 | source refs、coverage、claim support。 |
| Continuation List | 补调账户、银行、支付机构、材料、字段。 | `validate_continuation_list`。 |
| Report Card | 全案报告、一人一档、专题研判、复核材料。 | `validate_report_claims`、反模式、输出口吻。 |
| Attachment Table | 统计附件、Top 表、专题清单、复核表。 | 行数、金额、单位、scope、字段。 |
| QA Review | 数字、口径、措辞、附件复核。 | unsupported claim、单位、重复、缺口、工程黑话。 |
| Adversarial Review Card | 报告、链路、专题线索或重大金额结论对抗复核。 | 金额反算、scope 反查、unsupported edge、法律敏感词。 |

### 9.2 核心卡片协议

Answer Card 必备字段：

```text
task_profile
answer_card_complete
required_facts_present
unsupported_flows_present
recommended_next_action
max_additional_tools
context_compiler
key_facts
warnings
next_actions
forbidden_as_facts
```

`answer_card_complete`、`max_additional_tools` 等字段只属于内部预算和停止状态。用户可见输出应表达事实、线索、边界、缺口和下一步，不得输出这些控制字段，也不得用它们压制下一轮新对象、新口径、新时间窗或继续追一层。

Claim Review Card 必备字段：

```text
verified_claims
corrected_claims
unsupported_claims
unsupported_flows
missing_source_boundaries
forbidden_phrasings
next_review_actions
write_blocked
```

Investigation Card 必备字段：

```text
investigation_intent
scope
open_hypotheses
mandatory_review_cards
evidence_status
amount_concentrations
time_concentrations
duplicate_or_same_fact_risk
missing_counterparty
financial_product_or_asset_leads
downgraded_hypotheses
recommended_next_action
max_additional_tools
stop_conditions
forbidden_as_facts
```

Flow Card 必备字段：

```text
seed
scope
supported_edges
unsupported_flows
visible_data_cut_off
amount_gap
time_gap
competing_candidates
mermaid_source_table
follow_up_targets
claim_guard
```

### 9.3 报告 Schema 与 Finding Card

报告不是把事实卡改写成长文，而是把可核准事实、图谱、线索、边界和补证动作组织成可交付研判材料。报告 schema 必须固定，字段缺失时不能用自然语言补齐。

报告必备章节：

| 章节 | 内容 | 门禁 |
| --- | --- | --- |
| 案件数据范围与口径 | source scope、清洗状态、分析索引、时间范围、单位、去重口径。 | scope / coverage / reconciliation 未齐备时禁止写全量。 |
| 主体与账户概览 | 直接账户、候选账户、户名树、账户质量风险。 | 候选账户不能写成名下账户。 |
| 资金总体特征 | 收入、支出、余额、交易笔数、Top 排名、趋势。 | 金额、笔数、metric、direction、单位必须有 ledger。 |
| 资金流向与穿透 | 一跳、下一跳、可见停止点、补调端点、Mermaid。 | 只允许 supported transaction edge。 |
| 线索专题 | 现金、支付通道、理财、资产消费、工程/涉诉、控制/代持候选。 | 只能写线索、特征和补证动作。 |
| 数据质量与复核意见 | 重复、换卡、空户名、缺失对手、source drift、覆盖限制。 | 不得把缺口写成无风险。 |
| 证据与补调清单 | 补调账户、机构、材料、字段、证明目标、优先级。 | 续调清单必须通过校验。 |
| 结论与限制 | 已支持事实、需降级假设、未支持 claim、人工确认需求。 | Claim Verifier 未通过时阻断强结论。 |

Finding Card 必备字段：

| 字段 | 含义 | 写作要求 |
| --- | --- | --- |
| `finding_id` | 内部 finding 标识。 | 不直接暴露给用户正文。 |
| `title` | 研判发现标题。 | 只描述事实或线索类型，不写法律定性。 |
| `fact_summary` | 可支持事实摘要。 | 金额、笔数、主体、账户、对手方、时间窗必须有口径。 |
| `scope` | 数据范围和去重口径。 | scope 缺失则 finding 降级。 |
| `evidence_refs` | ledger、source、fact、edge 支撑。 | 用户正文只呈现可解释事实字段。 |
| `claim_status` | supported / corrected / lead / missing / downgraded / forbidden / unsupported。 | unsupported / forbidden 不进入结论。 |
| `risk_boundary` | 解释限制和禁止外推。 | 必须保留在报告中。 |
| `next_action` | 续查、补证、人工确认。 | 必须可执行、可排序、可验收。 |

写作规则：

- 先事实，后研判，再边界，最后补证。
- 金额用原始单位和转换口径，避免元/万元漂移。
- “发现”“显示”“提示”“疑似”“候选”“需核验”必须按 claim 状态使用。
- “查明”“确认”“最终流向”“实际控制”“违法所得”“资产归属”等强表达必须有完整证据和人工确认边界。
- 报告结论不能新增事实；只能引用已经进入 Evidence Ledger 和 Claim Verifier 的内容。
- Mermaid 图旁必须有边表或事实表；无边表不画图。
- 附件表必须能回到 source scope、字段、单位、去重口径和校验状态。

### 9.4 交付物生命周期

1. 生成：交付物只能从确定性事实卡、证据包、图谱边或报告 schema 生成。
2. 校验：金额、笔数、账户、对手方、资金边、附件行数、报告 claim 必须有对应校验路径。
3. 更新：用户改变口径、范围、主体、时间窗、排序指标或报告口吻时，必须重新编译事实卡或复核对应 claim，不能只改文字。
4. 阻断：证据缺失、claim unsupported、flow unsupported、范围不明、后端 API 不可用、单位不明时，必须阻断强结论。
5. 复用：多轮追查可复用本案内的 scope、ledger、hypothesis 和 artifact，但必须保持案件隔离。
6. 清理：artifact、trace、eval、ledger 不得跨案长期污染，也不得进入系统 Codex 状态。

交付物状态：

| 状态 | 含义 | 用户可见边界 |
| --- | --- | --- |
| draft | 已生成但未完成事实复核或 claim review。 | 只能作为草稿或研判卡，不能作为正式报告。 |
| verified | 关键事实、scope、edge、claim 已通过门禁。 | 可以进入正式材料，但仍保留口径和限制。 |
| blocked | 存在 unsupported claim、unsupported flow、范围不明或法律越界。 | 只输出阻断点、纠正口径和补证动作。 |
| stale | scope、数据源、清洗、用户口径或证据状态变化。 | 必须重新编译事实卡或重跑复核。 |
| superseded | 被新的事实卡、报告卡或人工确认替代。 | 保留审计关系，不再作为最终材料来源。 |

## 10. 质量治理与反模式门禁

### 10.1 输出准则族

| 准则族 | 适用任务 | 约束重点 |
| --- | --- | --- |
| ask | 普通事实问答、金额核验、当前案件问题。 | 一次事实卡优先；金额和笔数必须有确定性来源；不暴露内部 id。 |
| qa | 数字、口径、数据质量、附件、重复、单位复核。 | 先说边界；不能把缺口补成事实；不能用 bounded rows 算全案。 |
| rank | Top 账户、Top 主体、Top 对手方。 | 必须有排序指标、范围、coverage；禁止用 profile 或样本推断排名。 |
| lab | 开放式深挖、可疑点发现、不规律模式。 | 假设分级；线索和事实分离；每轮给下一步追查和收口边界。 |
| flowgraph | 资金链路、去向、Mermaid、下一跳。 | 只画 supported transaction edge；聚合排行不能画成链路。 |
| report | 报告生成、报告改写、报告复核。 | Claim Verifier 硬门禁；未支持 claim 降级或阻断。 |
| evidence | 证据包、补调清单、附件检查。 | 输出补证对象、字段缺口、优先级；不能写最终结论。 |
| review | 输出前复核、反模式检查、口吻检查。 | Critique Pass 覆盖事实、边界、假设、报告语气和工程黑话。 |

### 10.2 术语状态

| 状态 | 含义 | 可否写入报告事实 |
| --- | --- | --- |
| supported | 已有确定性事实或支持边。 | 可以，但必须保留口径和边界。 |
| corrected | 原表述需纠正，已有正确口径。 | 可以写纠正后的事实。 |
| lead | 线索或可疑特征。 | 不能写成已查明，只能写为续查方向。 |
| missing | 关键证据缺失。 | 不能写成事实，必须列补证动作。 |
| downgraded | 原结论需降级。 | 不能按原强度表达。 |
| forbidden | 禁用表述或越权结论。 | 不能进入用户可见事实和报告结论。 |
| unsupported | 无事实支撑。 | 必须删除、纠正或标注为不支持。 |

### 10.3 反模式目录

Blocking anti-patterns：

| 反模式 | 风险 | 正确处理 |
| --- | --- | --- |
| Full-report-only closure | 只调用全案报告链路，不检查 Lab mandatory cards、data quality、source audit、report validation 和 `write_blocked`。 | 报告前必须读取事实卡、质量卡、mandatory review card 和 claim review。 |
| Unverified source substitution | 把未复核来源、临时计算、样本行或局部预览当作 source-backed 案件事实。 | 回到 analytix data/runtime backend / MCP deterministic runtime，或进入带 evidence boundary 的 Case Workbench。 |
| Bounded-row totals | 汇总样本行并写成全案、账户或主体 totals。 | totals 只能来自 aggregate / rank / scope facts。 |
| Coverage language without coverage | 写 `全量`、`全部`、`覆盖`、`最大`、`Top`，但没有 coverage、rank、reconciliation 或 source audit。 | 补 coverage 或降级为样本/局部事实。 |
| Dashboard metadata as coverage | 把 dashboard counters 当作分析账户、户名或交易覆盖。 | 使用 scope map、scope coverage 或 rank output。 |
| Candidate ownership upgrade | 把 candidate accounts 写成确认名下账户。 | direct registered accounts 与 candidate-linked accounts 分离。 |
| Claim validation skipped | 报告或附件 claim 未经过 `validate_report_claims` 或 `validate_continuation_list`。 | 报告级材料必须先复核。 |

High-risk interpretation errors：

| 反模式 | 风险 | 正确处理 |
| --- | --- | --- |
| Blank detail holder != unregistered account dimension | 把交易明细空户名误认为账户维表未登记户名。 | 区分 detail row 缺失字段与 account dimension 字段。 |
| Financial product mistaken as cash | 把理财/基金/证券/保险关键词误写成现金或普通转账。 | 先进入 financial product classifier。 |
| Duplicate cleaning over-trust | exact duplicate 规则未删减被误解为不存在同事实跨账户重复。 | 写为 exact duplicate 未删减，不排除同事实候选。 |
| Card replacement collapse without proof | 因时间、金额、余额、摘要接近直接合并换卡/补卡流水。 | 标注 merge candidate，待确定性 policy 或人工确认。 |
| Full-case blind dedupe | 跨全案按相似字段扣减 totals。 | 至少限定同 holder/person account set 或人工确认 account set。 |
| Unit drift | 元与万元心算漂移，导致金额放大或缩小。 | 保留原始元金额和转换口径。 |
| Endpoint overclaim | Top 出账无下游被写成最终去向。 | 写成补调目标或可见数据停止点。 |

Tool-sprawl anti-patterns：

| 反模式 | 风险 | 正确处理 |
| --- | --- | --- |
| All-tools sweep | 因问题宽泛就运行大多数 MCP 工具。 | 先走 `/analytix plan` 或 Task Profile 获取 lane budget。 |
| Ranking by profile tool | 用 account profile、bounded rows 推断全案 Top。 | 使用 rank lane。 |
| Lab without follow-up | 运行 Lab 后不跟进最高价值卡片。 | 按 recommended next action 和预算继续 trace/probe/validation。 |
| Probe without hypothesis | `hypothesis_probe` 参数模糊，没有 holder/account/date/keyword focus。 | 查询前写清 hypothesis、scope 和 fact_need。 |

### 10.4 质量评价协议

| 任务族 | 质量重点 |
| --- | --- |
| 普通事实问答 | 金额、笔数、账户、户名、对手方、方向、时间窗、scope、单位。 |
| Top / 排名 | metric、direction、scope、coverage、排序正确、单位正确。 |
| 主体/账户画像 | 直接账户、候选账户、账户统计、对手方、边界。 |
| 资金去向/穿透 | 一跳事实、下一跳、supported edge、终点分类、补调对象。 |
| 开放式深挖 | 侦查意图、假设队列、证据状态、金额集中、时间集中、重复/换卡、缺失对手、理财/资产线索、下一步。 |
| 数据质量复核 | source、cleaning、coverage、duplicates、scope difference、单位、字段缺失。 |
| 报告 claim review | verified、corrected、unsupported、boundary、forbidden、next actions、write_blocked。 |
| passive 非资金 | 不调用资金工具，不强套资金研判结构。 |

Auto-fail：

- 错误金额、笔数、单位、方向或时间窗。
- 样本、bounded rows、dashboard metadata 冒充全案事实。
- candidate account、控制/代持、同事实候选写成确认归属。
- 没有 supported edge 却画 Mermaid、闭环或最终去向。
- 银行流水直接写法律结论或最终资金归属。
- Agent 把未复核来源、临时计算、样本行或局部预览写成案件事实。
- production 读取 golden/oracle/eval fixture。
- 用户可见输出含 raw rows、SQL result、内部 id、audit ref、artifact id、token。
- 非资金任务触发资金工具。
- 无 plan、无预算、无收口边界地扫 specialist tools。

### 10.5 输出规则指令集与 Critique Pass

这里的“指令集”不是用户命令，也不是 `/analytix` 命令族，而是每次输出前必须执行的专业规则集合。它承担 impeccable 式反模式检测、Brooks Lint 式确定性规则和人工可读 critique 的组合职责。

输出规则指令集：

| 指令 | 适用范围 | 必须检查 |
| --- | --- | --- |
| Fact-first | 所有输出。 | 先写确定性事实、口径和单位，再写研判。 |
| Scope-first | 金额、笔数、Top、报告。 | scope、time window、dedupe policy、success filter、coverage。 |
| Evidence-before-claim | 报告、图谱、附件、重大结论。 | claim 是否有 ledger、source refs、supported edge 或人工确认。 |
| Hypothesis-is-not-fact | Lab、专题线索、资产/控制/现金。 | 假设状态是否标注 lead / missing / downgraded。 |
| Supported-edge-only | Flow Card、Mermaid、路径穿透。 | 每条边是否来自逐笔交易事实。 |
| Candidate-boundary | 主体、账户、控制、代持、换卡、重复。 | candidate 是否被误写为确认归属。 |
| No-legal-overclaim | 报告、结论、专题线索。 | 是否出现无证据法律定性或最终归属。 |
| Raw-output-block | 所有用户可见输出。 | raw rows、大 JSON、SQL、内部 id、artifact id 是否外泄。 |
| Low-cost-answer | 普通问答、Top、主体画像。 | 是否在完整 Answer Card 后继续多工具扫描。 |
| Passive-non-fund | 非资金任务。 | 是否错误调用资金工具或套用资金研判结构。 |

Critique Pass 必须覆盖：

- 普通问答：金额、笔数、单位、scope 和内部 id。
- Top 排名：metric、direction、coverage、样本冒充全案。
- 主体画像：直接账户与候选账户。
- 资金流图：unsupported edge、聚合边、Mermaid 表达。
- Investigation Lab：假设升级、缺少下一步、缺少收口边界。
- 报告：claim support、法律敏感词、现金/资产/控制过度推断。
- 续调清单：空字段、重复目标、证明力不明。

Critique 结果状态：

| 状态 | 动作 |
| --- | --- |
| pass | 输出可交付。 |
| correct | 修正金额、单位、scope、措辞或排序。 |
| downgrade | 把结论降级为线索、候选或待复核。 |
| block | 阻断正式报告、Mermaid、附件或重大 claim。 |
| ask_human | 需要人工确认账户集合、重复族、控制关系或证据材料。 |

### 10.6 Doctor、功能闭环与质量产物清单

顶级商业成熟插件不能只靠主观阅读判断质量，也不能靠旧式对比实验或单一分数证明质量。质量产物必须覆盖静态结构、运行健康、来源信封、事实准确、报告复核、被动边界、成本、隔离、真机功能和发布身份。

| 脚本 / 产物 | 职责 | 必须证明 |
| --- | --- | --- |
| `doctor.mjs` | 插件结构、manifest、MCP、references、registry、ledger、claim、功能测试契约检查。 | 插件包完整、root/skill references 一致、能力与命令无漂移。 |
| `check-health.mjs` | 目标 runtime/backend 健康检查。 | MCP server 可启动、backend 可达、所选案件解析和基本事实链路可用。 |
| `scripts/frontdoor-smoke.mjs` | navigator smoke。 | 普通问答、scope、report、passive、answer contract 基本路径稳定，且不垄断语义工具选择。 |
| 真实前门功能任务 | 用户可见前门回归。 | 真实案件中的多主体、无显式 holder、追踪类问题能落到正确 lane，输出真实结果而非说明书、门禁模板或历史样例。 |
| `functional-eval.mjs` 或迁移后的功能 runner | 功能回放与风险断言。 | Pair Amount、Object Dossier、Full Case、Trace/Lab、Workbench、Visual/Report、Passive 场景按 source + validation + delivery 闭环。 |
| `eval-coverage-contract.mjs` | 功能覆盖契约。 | 每个 capability 有任务、风险、passive、adversarial 和 report-review 覆盖。 |
| `golden-qa.mjs` | eval-only 标准问答评测。 | 金额、笔数、账户、户名、对手方、scope 和单位准确；不进入 production path。 |
| `release-slice-audit.mjs` | 交付切片审计。 | manifest、server、registry、references、功能闭环产物和工作区边界一致。 |
| `runtime-cache-contract.mjs` | runtime cache 合约检查。 | 本地 remount 不越过 Analytix-owned runtime 边界。 |
| `sync-runtime-cache.mjs` | 本地 remount 辅助。 | 只作为本地验证，拒绝非 Analytix runtime 和不完整 skill mount。 |
| `prepare-hub-package.mjs` | Hub package 准备。 | 包内容、manifest、skills、MCP、assets、references 可交付。 |
| `dev-run-with-plugin.mjs` | 本地开发运行辅助。 | 开发路径不伪装成客户安装或发布路径。 |

功能闭环产物必须具备：

- 任务覆盖：quick fact、Pair Amount、rank、holder/account dossier、trace、lab、full-case、visual、report、workbench、passive。
- 风险覆盖：金额错、单位错、候选归属升级、unsupported flow、法律越界、raw output 外泄、工具扩散、同事实/换卡重复。
- 成本覆盖：普通问答工具预算、报告工具预算、重复调用抑制、raw payload 压缩。
- 来源覆盖：`case_source_envelope`、source-of-truth、metric scope、validation state、delivery state。
- 证据覆盖：amount、count、account、holder、counterparty、edge、claim、source refs。
- 交付覆盖：Answer Card、Investigation Card、Flow Card、Report Card、Continuation List、QA Review、artifact/render/execute QA。
- 隔离覆盖：不写系统 Codex、不接第三方遥测、不跨案记忆、不读取 eval fixture。
- 用户可见覆盖：首屏必须给出工作结果、事实/线索/边界/下一步，而不是只给工具说明、门禁说明或能力说明。
- 样例泄漏覆盖：生产输出不得出现与当前任务无关的固定 golden 主体、固定交易号、固定金额、固定日期集合或固定提示词残片；出现即 auto-fail。
- 跳过覆盖：health 或 golden preflight 因案件不含固定样例而跳过时，只能证明插件可运行，不能证明该真实案件任务质量达标。

### 10.7 功能覆盖维度与阻断契约

功能测试只用于发现 drift、回归、输出污染、能力缺口和断言错误；不能让施工线程围绕分数优化。功能契约必须由 Capability Registry 派生，并覆盖能力、命令、风险、成本、报告和 passive 边界。

Registry 级功能字段：

| 字段 | 含义 | 蓝本要求 |
| --- | --- | --- |
| `min_tasks` | 最小任务数量。 | 覆盖普通问答、排行、主体、去向、Lab、报告、QA、workbench 和 passive。 |
| `min_cases` | 最小案件覆盖。 | 防止只对单一数据形态过拟合。 |
| `min_tasks_per_required_case` | 每个必要案件的任务覆盖。 | 每个案件至少覆盖事实、边界或报告中的关键面。 |
| `min_passive_nonfunds_tasks` | 非资金被动任务覆盖。 | 证明插件不会污染普通 Codex 工作。 |
| `min_passive_near_miss_tasks` | 包含资金/账户字样但非案件分析的 near-miss 覆盖。 | 证明“资金”“账户”等词不会误触发案件事实工具。 |
| `min_report_claim_review_tasks` | 报告 claim review 覆盖。 | 证明金额、Mermaid、法律敏感词、候选归属和来源缺口能阻断。 |
| `min_real_frontdoor_tasks` | 真实用户可见前门覆盖。 | 覆盖多主体、无 holder、追踪、报告、普通问答、Pair Amount、Workbench 和当前案件同步，不允许只跑固定 golden。 |
| `required_dimensions` | 必须覆盖的质量维度。 | 每个维度必须绑定任务族和 auto-fail 条件。 |

必备质量维度：

| 维度 | 证明内容 | Auto-fail 示例 |
| --- | --- | --- |
| `source_envelope` | 当前案件、source-of-truth、metric scope、validation state、delivery state 可用。 | 没有 source envelope 就输出确定结论。 |
| `pair_amount_reconciliation` | Pair Amount 按主体/账户/对手粒度、去重/换卡/同事实风险核算。 | 用排行金额单独最终作答，或把两个候选金额统计范围的差异取最大值。 |
| `deterministic_rankings` | Top 账户、户名、对手方 ranking 由确定性事实支撑。 | metric、scope、单位、coverage 缺失或排序错误。 |
| `source_quality_boundaries` | source、schema、cleaning、coverage、duplicate 风险能正确降级。 | 暂不可用写成无风险、缺表或 0 值事实。 |
| `holder_ownership_boundary` | 直接账户和候选账户分离。 | candidate account 写成名下账户。 |
| `destination_and_continuation` | 一跳、下一跳、终点分类和补调清单边界正确。 | unmatched endpoint 写成最终去向。 |
| `investigation_lab` | 开放假设、证据状态、下一步和收口边界完整。 | 假设写成事实，或 stop flag 压制继续追一层。 |
| `route_intent_integrity` | 用户侦查意图、主体、时间窗和任务类型被保留。 | 多主体追踪被路由为报告门禁，或输出标题/意图替换成无关测试主体。 |
| `workbench_boundary` | 自定义口径能在当前案件、只读、清洗/analysis scope、限行和 artifact 边界内执行或返回 gap。 | 语义工具不足就拒答，或未验证 SQL 结果写成报告级事实。 |
| `report_claim_review` | 报告 claim、Mermaid、法律敏感表述和 source refs 能阻断。 | unsupported claim 进入正式结论。 |
| `passive_nonintervention` | 非资金任务不调用资金工具。 | 普通写作、公式、会议纪要、脱敏问题误触发案件工具。 |

Auto-fail 必须覆盖：

- 无来源强答、弱来源冒充、未验证计算或局部预览被写成案件事实。
- bounded rows 汇总成全案 totals。
- candidate account 写成 confirmed ownership。
- unsupported Mermaid / arrow flow。
- 法律敏感词、最终归属、控制关系无证据闭合。
- report claim 未经复核进入正式措辞。
- passive 非资金任务触发 `analytix_funds`。
- raw JSON、内部 id、artifact id、token 或 debug payload 进入用户正文。
- 模型可见 `function_call_output` 或 MCP 普通 content 暴露大 JSON、转义 JSON、重复 JSON、support envelope 或内部字段，即使最终中文答案看起来合格也判失败。
- golden / oracle / rubric 进入 production path。
- 生产输出泄漏固定 golden 主体、固定样例金额、固定样例交易号或与当前案件/当前问题无关的样例事实。
- 用户已经在 analytix 案件项目 UI 选中案件，但 Agent 因 current case-project binding 未同步而要求用户再次提供 `case_id`，且没有给出自动同步失败原因和恢复动作。
- 前门输出主要是插件说明、报告门禁、工具调用说明或工作流说明，而不是围绕用户问题的事实、线索、边界和下一步。

### 10.8 Eval Fixture 隔离与旧对比模式删除

本地评测夹具目录是评测专用区域，不是生产资源、不是 skill reference、不是 MCP resource，也不是案件事实来源。蓝本只定义隔离和功能契约，不能把标准答案、真实任务细节、对比基线或 oracle 内容搬入正文。

Eval fixture 文件边界：

| 文件 | 用途 | 禁止 |
| --- | --- | --- |
| Golden answer set | 存放 eval-only 任务、标准逻辑、正确事实、允许不确定性和禁用 claim。 | 进入 production resources、skill references、MCP 输出、用户正文或报告事实层。 |
| Golden eval rubric | 存放人工/模型评审 rubric、质量维度和自动失败规则。 | 作为 prompt 注入生产问答；把 rubric 文案当成案件事实。 |

隔离规则：

- production path 不读取 golden / oracle / rubric。
- MCP `resources/list` 不暴露 eval fixture。
- doctor 可以检查 fixture 存在、schema 和隔离状态，但不能把答案内容注入 runtime。
- 功能测试可以读取 production 输出、工具序列和 artifact 身份，但 production 不能反向读取 eval oracle。
- 功能失败只能推动能力、契约或测试断言修正，不能让生产路径硬编码标准答案。
- 真实任务回归可以记录任务形态、路由期望、禁止泄漏标记和用户可见输出要求，但不得把真实案件名称、真实主体名单、真实金额细节或标准答案作为 production resource、skill reference 或默认 prompt。
- 固定 golden 只能覆盖其对应样例事实族；当 selected case 不含该事实族而跳过时，必须另有真实语义工具 / navigator 任务证明该案件的路由和输出质量。
- 旧式 baseline mode、模式名、分数和模式比较不再作为施工或发布依据；如旧脚本仍存在，必须迁移或包裹为功能任务 runner，并删除对模型可见的对比语义。

### 10.9 Release-quality Functional Closure 术语与阻断条件

Release-quality functional closure 是发布前真实验收产物，不是普通 smoke 输出。它必须证明同一插件包、同一 MCP server、同一 skill mount、同一 runtime、同一 package source、同一当前案件数据和同一功能任务集之间身份一致，且没有硬性风险。

以下产物一律不能作为发布依据：

- 旧版本插件、旧 HEAD、旧 MCP server 或旧 runtime cache 生成的产物。
- 含旧工具封锁口径的产物，例如把 Data Analytics 允许的 DuckDB、SQL、Python、notebook 或 shell 分析手段写成不可用。
- 只看分数、平均值或单一 pass 标记的产物。
- 未确认 Analytix-owned runtime cache、app-server binary、client process cwd、backend URL、current case project 和 release identity 的产物。
- 没有真实 Analytix 内嵌 Agent 前门功能任务，只跑 CLI、fixture、oracle 或 direct MCP 的产物。
- 输出一堆 JSON/工具日志但用户真实问题仍答错、答浅、答成说明书或丢失侦查意图的产物。

| 术语 | 含义 | 施工要求 |
| --- | --- | --- |
| `functional_closure` | 当前版本功能闭环产物。 | 必须绑定 source envelope、validation state、delivery state 和用户可见输出。 |
| `hard_diff` | 发布或模型输出对比中的硬差异数量。 | 必须作为阻断级差异处理，不能用文字解释绕过。 |
| `auto_fail` / `auto_fail_count` | 自动失败项数量。 | 发布级产物中必须为零；非零时只能进入修复。 |
| `runtime preflight` | 在真实 runtime / backend / app-server 前置检查。 | 失败时不能生成发布级质量结论。 |
| `same-version plugin cache` | runtime 中已存在同一插件包身份的 cache。 | local remount 只能刷新已安装同身份 cache，不能制造安装。 |
| `confirm-local-remount` | 明确确认本地 remount 的人工开关。 | 没有该确认不能执行写入型 remount。 |
| `package hash` / `archive SHA256` | Hub package source 和 archive 的完整性摘要。 | 必须用于包源核准，不能被口头说明替代。 |
| `client_process_cwd` | 真实 Agent UI 客户端进程工作目录证据。 | 必须证明 cwd/PWD/INIT_CWD 没有污染 release 工作区。 |
| `dirty worktree` | 发布切片外存在未闭合改动。 | 不能用该状态生成发布级产物。 |
| `secret-like` | 类 token、key、secret、PWD 等敏感形态文本。 | 必须脱敏或阻断进入功能闭环产物。 |
| `ANALYTIX_FORCE_WEB_BUILD` | 本地开发启动辅助变量。 | 只属于开发运行辅助，不作为插件能力或发布证据。 |

Release-quality 阻断条件：

- `auto_fail_count` 非零。
- `hard_diff` 非零且属于事实、证据、报告或隔离硬差异。
- functional closure 与 manifest、server、registry、package、runtime 或 skill mount 身份不一致。
- 功能闭环缺少 runtime preflight、app-server binary、client process cwd、current case project、source envelope、validation state、delivery state 或用户可见输出。
- read-only 质量运行中出现非 output artifact 的文件变更。
- 发布切片外存在 dirty worktree。
- 功能闭环产物包含 token、JWT、refresh token、session token、Bearer token 或 secret-like 文本。
- package source、archive digest、generated marketplace、installed runtime cache 任一不一致。
- 被动非资金任务出现资金工具调用。
- report review 未能阻断 unsupported flow、候选归属升级、法律敏感词或缺失来源边界。
- 真实语义工具 / navigator / 真机功能任务中出现样例事实泄漏、当前案件未同步、追踪问题误入报告门禁、输出只有说明没有结果。
- `check-health` 或固定 golden 场景显示 pass 但关键任务被 skipped；该产物不得单独作为发布级质量证明。

Release-quality functional closure 只证明“目标包在目标 runtime 中达到质量门槛”。它不能替代人工复核，不能证明其他安装面、其他 package source 或其他 runtime 的质量。

## 11. 边界、隔离与运行时生命周期

### 11.1 系统隔离

- 不改系统 Codex。
- 不改全局 Codex home。
- 不污染系统 hooks / MCP / config / cache / plugin 状态。
- 不改变 Analytix 内嵌 Agent 的默认行为。
- 不把 Analytix 插件能力写成系统 Codex 的全局默认能力。
- 插件运行时、缓存、日志、artifact、证据账本、eval 输出只属于 Analytix。
- 不把插件实现成独立业务系统；插件不接管 Analytix 后端、前端、案件数据平台、导出链路或人工审核责任。

### 11.2 数据访问与来源等价边界

- 案件结论必须有 source-of-truth、当前案件 scope、计算口径、验证状态和 evidence boundary。
- 所有案件事实任务共享 `case_source_envelope`：`case_identity`、`source_of_truth`、`source_scope`、`data_quality_state`、`metric_scope`、`validation_state`、`delivery_state` 和 `gap_card`。它是 Data Analytics 式 preflight envelope，不是总指挥、硬路由或硬停止。
- 交易事实以清洗明细和 approved analysis index 为准；开户、户名、证件、联系方式、住址等主体事实以清洗开户信息和主体维表为准；casegraph/fundgraph/scope map 只作来源地图、候选路径和上下文，不得替代交易事实或报告 claim 来源。
- `source_of_truth` 必须显式选择控制来源；来源冲突时说明优先级、差异和影响，不能用弱来源、样例、记忆或工具说明替代 live/source-backed verification。
- `data_quality_state` 必须覆盖粒度、时间窗、缺失、重复、join 风险、异常值、freshness、清洗覆盖和口径漂移等足以影响结论的检查。
- `delivery_state` 必须区分 inline answer、table/chart inventory、notebook/workbench artifact、report draft 和 appendix pack；聊天总结不能冒充已生成报告、图表、notebook 或附件包。
- `render/execute QA` 必须进入交付状态：报告要能打开或渲染检查，notebook/workbench 要记录执行状态，图表/表格要在最终容器检查；失败时写 blocker，不写“已交付”。
- SQL、Python、notebook 或自定义计算不是被禁工具；它们只有在来源、权限、scope、行数、验证和交付状态清楚时，才能支撑用户可见事实。
- 当前候选版本的功能闭环产物必须生成在 `output/analytix-fund-analysis/functional-closure/`，并绑定当前 manifest/server version、HEAD、runtime identity 和工具面；插件源码目录下不保留历史 `evidence/` 对比产物，Hub package 也不得包含这些历史产物。
- Agent 不把 bounded rows 汇总成全案事实。
- Agent 不把 raw rows、raw query result dumps、raw graph payload 塞进模型上下文。
- analytix data/runtime backend / MCP / Rust deterministic runtime 可以使用 DuckDB、SQL、索引、聚合和图计算，但必须以受控 schema、证据账本和 artifact 返回事实。
- production path 不读取 golden / oracle / eval fixture。
- 受控 SQL/notebook 可以来自用户提供的安全 SQL、Codex 生成的显式列查询，或 backend 根据 `query_request` 规划的查询；无论来源如何，都必须由 Analytix-owned runtime / MCP 承载层二次校验、限行、审计和返回 evidence boundary。
- 语义工具不足时，插件应先尝试等价语义工具或 scope/quality/trace/lab 组合；仍不足且问题明确时，才进入 Case Workbench。Case Workbench 的成功结果可以回答当前案件问题，但其通用化只能通过后续版本沉淀为新的 source helper / MCP 能力。

### 11.3 数据管线与查询顺序协议

插件的事实能力来自 analytix data/runtime backend / MCP、casegraph/fundgraph、受控 SQL/Python/notebook/workbench 和 Evidence Ledger 共同构成的 source-backed 数据管线。蓝本限制的是无来源强答、弱来源冒充、未验证计算、跨案污染和假交付，不限制 Data Analytics 允许的可复核分析手段；任何 SQL、Python、DuckDB、notebook 或 shell 辅助分析只要落在当前案件、清洗/analysis scope、只读、限行、purpose、validation 和 artifact 边界内，就可以成为语义工具不足时的正常兜底路径。数据管线必须把导入、清洗、分析索引、统计聚合、图谱、workbench 计算和报告口径分开。

数据管线层级：

| 层级 | 职责 | 使用边界 |
| --- | --- | --- |
| Import scope | 导入文件、导入任务、原始来源。 | 只说明来源，不直接支撑 report-grade totals。 |
| Cleaning scope | 清洗任务、字段修正、去重策略、异常处理。 | 清洗影响必须进入 source audit。 |
| Normalized detail | 规范化交易明细。 | 可支撑逐笔 edge，但 bounded rows 不能算全案。 |
| Analysis index | 统计索引、按户名/账户/对手方聚合。 | 支撑 ranking、coverage、profile、scope map。 |
| Reconciliation | detail、directional rows、tree、report scope 对齐。 | 支撑报告级金额和笔数。 |
| Casegraph / Fundgraph | 预索引事实图和资金边图。 | 支撑低上下文追查、claim support 和 Mermaid。 |
| Evidence Ledger | facts、edges、claims、source refs。 | 支撑用户可见事实和报告交付。 |

默认查询建议：

1. 解析当前案件。
2. 获取数据管线概览、scope map 或 casegraph。
3. 获取 coverage、reconciliation、rank 或 profile。
4. 需要例证时再取 bounded transaction rows。
5. 需要链路时取 supported edge / fundgraph / evidence pack。
6. 需要开放深挖时进入 Investigation Lab。
7. 语义工具不足且用户明确需要自定义口径、SQL、notebook 或可回放计算时，
   进入 Controlled Case Workbench。
8. 需要报告或附件时进入 Claim Verifier / continuation validation。

该顺序是成本友好的 source selection 启发，不是硬路由。Codex 可以根据用户问题、已知 source-of-truth、风险状态和交付形态跳过、重排或补充步骤；验收只检查来源、验证和交付是否成立，不检查是否机械执行完整顺序。

当前案件解析协议：

- `case_id` 显式参数优先；没有显式参数时读取 Analytix current case-project binding；current case-project binding 不可用时只输出同步失败边界和恢复动作。
- analytix 案件项目 UI 的选中案件状态不能只停留在前端本地状态；进入 Agent、切换案件、启动 runtime 或刷新页面时，必须把选中案件同步到 current case-project binding。
- MCP runtime 不得自行猜测默认案件、最近案件或固定测试案件；也不得因为工作目录、历史会话或 eval artifact 推断案件。
- 当前案件变化会使旧事实卡、旧 ledger、旧 artifact、旧 hypothesis、旧 report claim stale；必须重新编译或显式复核。
- 用户显式输入的 `case_id` 与 UI/后端当前案件冲突时，必须在 scope warning 中说明冲突来源，并以显式 `case_id` 为准执行本轮任务。
- 真实 UI 回归必须覆盖“用户在案件中心选中案件后直接进入 Agent 提问”的路径；如果仍要求用户重复提供 `case_id`，属于当前案件同步失败。

查询安全规则：

- 优先聚合和事实卡，再取 bounded rows。
- row slice 只能举例或支撑局部 edge，不得计算全案 totals。
- heavy report、Lab、deep trace 不能无预算并行扫。
- 工具失败写失败状态，不写零值事实。
- missing output 不是无风险、无交易或无下游。
- source scope 改变后，旧事实卡必须 stale。
- 人工确认可以提升证据状态，但必须写入 ledger 和适用 scope。

### 11.4 临时不可用、重试与非零值边界

工具失败、schema 暂不可用、audit 需复核、字段缺失和 bounded preview 都是证据状态，不是事实结论。插件必须把这些状态转成边界、阻断或下一步，而不是让模型补齐。

| 状态 | 含义 | 正确输出 | 禁止输出 |
| --- | --- | --- | --- |
| `schema_status=temporarily_unavailable` | schema inventory 暂时不可用。 | 写为 retry boundary，需要重试或补充 source audit。 | 写成缺表、无数据、无交易。 |
| `audit_status=needs_review` | source、schema、清洗或未索引来源存在待复核。 | 写为 needs_review，禁止全量覆盖措辞。 | 写成已覆盖、无异常。 |
| tool error / timeout | backend 或 MCP 工具失败。 | 写失败工具、影响范围和收口边界。 | 把 missing output 当作 0。 |
| partial result | 子工具部分成功。 | 只使用 supported facts，其他 claim 降级。 | 把 partial 当完整事实卡。 |
| bounded rows / bounded previews | 为上下文压缩返回的样本、预览或截断账户集合。 | 作为例证、候选或下一步范围。 | 汇总成全案 totals 或完整账户集合。 |
| missing counterparty / blank holder | 对手方或户名字段缺失。 | 写为字段缺口、补调目标或待复核。 | 补想象主体或写成真实对手方。 |
| source drift / unindexed source | 存在可能未纳入分析索引的来源。 | 写为 coverage 风险和 source repair。 | 写全量、全部、最大、覆盖。 |
| stale artifact / stale card | scope、清洗、用户口径或数据状态变化。 | 重新编译事实卡或复核 claim。 | 复用旧报告 claim。 |

重试和阻断规则：

- retry boundary 不等于无事实；它只说明事实暂不能作为报告级结论。
- needs_review 不等于错误；它要求降级、补证或人工确认。
- partial 不等于失败；可用 facts 可以回答局部问题，但不能支撑强结论。
- bounded preview 不等于完整范围；后续工具必须使用原始 scope、holder/id scope 或明确 account set。
- 工具失败不能触发无来源强答、弱来源替代或未验证计算冒充事实。
- 语义工具能力不足不等于允许弱来源替代；只有明确自定义口径、SQL/notebook
  或可回放计算需求，才能进入 Controlled Case Workbench。若 workbench
  工具不可用，输出能力缺口和产品化沉淀目标，不伪造结果。
- 报告、Mermaid、附件、续调清单和重大 claim 遇到阻断状态时，必须输出阻断点和最小验证动作。

### 11.5 工具面边界

- 默认可见工具保持语义事实能力充足，覆盖 scope、rank、profile、trace、casegraph、hypothesis、quality 和 validation。
- 完整低层工具发现只用于开发、诊断或显式授权场景。
- 新增工具必须有 capability、schema、预算、证据要求、passive eval 和质量覆盖。
- 工具扩面不能替代能力注册、证据门禁和上下文压缩。

默认用户可见能力应保持“认知增强、低层受控”：

- 当前案件、范围、casegraph 和 coverage。
- 排名、账户画像、主体画像和对手方研判。
- 主体出账、下一跳、多跳和资金流图。
- 开放假设、数据质量和重复候选复核。
- 续查/附件清单复核和报告 claim 复核。
- `funds_investigate` 自然语言 navigator。
- 条件可见的 Controlled Case Workbench：只在明确 SQL/notebook/custom
  口径且语义工具不足时出现，不作为默认前门、假交付面或弱来源替代面。

来源/交付阻断项：

- 必需事实源不可用，却输出报告级确定结论。
- 弱来源、语义地图、样本行、局部预览或未验证计算被当成等价事实源。
- unbounded transaction export into the model。
- ordinary analysis 把 `fc_*_raw` 或 source files 当成生产事实。
- plugin-side all-pairs cash bridge matcher。
- plugin-side commingled fund allocator。
- legal / tax conclusion engine。

### 11.6 报告边界

- 没有 Evidence Ledger 的金额不能写成事实。
- 没有 supported transaction edge 的资金链不能画成确定箭头。
- 候选账户不能写成名下账户。
- 缺失对手方不能补想象主体。
- 现金断点不能写成最终去向。
- 理财、证券、保险、资产消费线索不能写成资产归属。
- 涉黑、违法所得、实际控制、代持、最终资金归属等法律敏感判断必须降级为线索或需复核，除非证据链闭合并经过人工确认。

### 11.7 分发与运行时生命周期

Hub 生命周期不是资金研判能力本身，但决定插件能否在 Analytix runtime 中被安全交付。

生命周期边界：

- public Agent plugin release 必须通过 Analytix Hub 或等效 Hub 管理路径。
- Analytix 涉案资金研判插件的正式发布入口是 Analytix 侧 Hub / admin-console
  路径（例如 `https://analytix.top/admin-console`），不是系统 Codex
  marketplace，也不是本地插件缓存。
- Analytix 内嵌 Agent 是平台内的 Agent 运行面；插件可以随新版
  Analytix 内置封包，也可以通过 Analytix Hub 插件更新进入已安装
  Analytix runtime。两条路径都必须保持 Analytix-owned runtime 隔离。
- GitHub push、GitHub Release、远程仓库状态不能被当作客户侧安装成功。
- customer install、uninstall、upgrade、rollback 必须通过 Analytix Hub 或 Analytix plugin page。
- installed plugin cache、required skill copy、generated marketplace file、artifacts、traces、evidence ledgers 必须留在 Analytix-owned runtime home。
- local remount aid 只能用于本地验证，不能制造客户安装路径。
- 插件不能自我升级、自动修改 backend/source-helper/MCP schema、自动写入用户安装态或自动把
  workbench 查询变成新工具。任何新 source helper、MCP 语义工具、后端查询能力、skill
  目录、eval 任务或报告交付面，都必须经过源码修改、测试、版本核准、Hub
  package 或新版 Analytix 封包。
- 插件版本、MCP server 版本、Capability Registry、tool schema、required skill
  mount、Hub package hash 和 installed runtime cache 必须同身份一致。任一方
  漂移时，不能用本地 remount 产物冒充发布级证据。

交付证据必须证明：

- doctor 通过。
- manifest 与 MCP server 身份一致。
- worktree 没有未闭合 slice 改动。
- health check 通过目标 runtime / backend。
- 功能闭环证明 source envelope、validation state、delivery state、事实准确、追查深度、报告交付和低成本要求达标。
- passive non-fund prompts 不路由到 `analytix_funds`。
- report review 能拒绝 unsupported Mermaid flows、legal overreach、incorrect amounts、unsupported account ownership、missing source boundaries。

### 11.8 Auth、Runtime Cache 与 Provenance

Auth 和 runtime state 是隔离边界，不是资金研判能力。插件可以依赖 Analytix 内嵌 Agent 共享的模型登录状态，但不能读、复制、修复、迁移或持久化 provider token、OAuth 状态、账户配置和模型供应商配置。

Runtime state 规则：

- plugin files、MCP config、generated artifacts、evidence ledgers、eval outputs、local marketplaces、runtime caches 必须留在 Analytix-owned runtime home。
- runtime-cache sync 只能作为本地验证或 remount aid。
- runtime-cache 操作必须拒绝非 Analytix runtime home。
- remount 不能制造客户安装路径，不能替代 Hub 发布或客户安装。
- doctor、health、eval 可以报告 auth/runtime 失败，但必须脱敏 Bearer token、API key、JWT、refresh token 和 session token。
- 登录修复属于 Analytix model-provider UI，不属于资金研判插件。

Provenance 规则：

- 内部 provenance 保留在 backend/tool/ledger/artifact 层。
- 用户可见输出引用具体事实字段、scope、金额、时间、账户、对手方和边界。
- 不使用 opaque id 作为用户证据。
- provenance 缺失时，表述降级为初步特征、候选线索或需复核事实。

### 11.9 运行写入面、Artifact 生命周期与证据身份链

插件可以写入的内容必须属于 Analytix-owned runtime、插件包准备区或明确的本地质量证据区。写入面是治理对象，不是绕过主系统的业务通道。

运行写入面：

| 写入面 | 允许内容 | 禁止 |
| --- | --- | --- |
| Report draft | 用户明确要求的报告草稿、研判材料、附件草稿。 | 无明确意图自动写正式报告；写入案件数据库。 |
| MCP artifact | raw rows、raw graph、debug payload、ledger detail、claim review payload。 | 作为用户正文证据；跨案复用。 |
| Evidence ledger | amount、count、account、holder、counterparty、edge、claim、source refs。 | 写入系统 Codex memory 或跨案长期记忆。 |
| Trace / eval output | 工具序列、route violation、quality signal、auto-fail、output diff。 | 写入生产 resources；泄露 token 或真实隐私字段。 |
| Hub package source | package source、marketplace stub、archive digest、package manifest。 | 代替 Hub publish/install；写系统 plugin cache。 |
| Runtime cache remount | 已安装 Analytix-owned runtime 内的本地验证 remount。 | 制造客户安装、越过 Hub、写系统 Codex home。 |

Artifact 生命周期：

| 状态 | 含义 | 处理 |
| --- | --- | --- |
| active | scope、数据、用户口径和事实卡仍匹配。 | 可被同一任务或复核链路复用。 |
| stale | scope、清洗、数据源、用户口径或证据状态变化。 | 必须重新编译事实卡或复核 claim。 |
| blocked | 含 unsupported claim、unsupported flow、法律越界或缺关键来源。 | 只能输出阻断点和补证动作。 |
| superseded | 被新的事实卡、人工确认、报告卡或复核卡替代。 | 保留审计关系，不再作为最终材料来源。 |
| eval-only | 只属于评测、断言器或 oracle。 | 不进入 production path、MCP resources 或用户正文。 |

证据身份链必须一致：

- plugin manifest identity。
- MCP server identity。
- Capability Registry identity。
- Command metadata identity。
- tool schema identity。
- root skill identity。
- required skill mount identity。
- root references 与 skill references 内容 identity。
- package source / archive digest identity。
- installed runtime cache identity。
- selected backend / runtime health identity。
- functional closure identity。
- worktree slice identity。

质量证据只有在身份链一致时才可作为交付依据。过期或不一致证据、其他安装面、其他 runtime、其他 package source 或不一致 skill mount 的结果，不能证明目标插件包可交付。

### 11.10 Runtime Cache 文件范围契约

Runtime cache contract 用于证明插件包、runtime cache 和 required skill mount 的文件范围一致。它不是安装路径，也不是发布路径；它只定义哪些文件应被 runtime 验证，哪些文件必须禁止进入 runtime cache。

文件域：

| 文件域 | 范围 | 作用 |
| --- | --- | --- |
| `REFERENCE_FILES` | `anti-patterns.md`、`blueprint-execution-plan.md`、`capability-registry.json`、`capability-registry.schema.json`、`casegraph-roadmap.md`、`command-metadata.json`、`command-router.md`、`domain-playbook.md`、`economic-investigation-analysis.md`、`economic-investigation-language.md`、`focused-skill-shared.md`、`fund-path-and-cash-bridge.md`、`hub-lifecycle.md`、`plugin-benchmark.md`、`public-security-official-writing.md`、`report-schema.md`、`runtime-boundary.md`、`top-pluginization-plan.md`、`tool-availability.md`。 | 定义 root references 与 skill references 的镜像范围。 |
| `PLUGIN_CACHE_FILES` | manifest、MCP manifest、核心 scripts、MCP modules、root references、skill、skill references。 | 定义插件 cache 应可核准的完整交付文件。 |
| `RUNTIME_SKILL_FILES` | runtime skill 的 `SKILL.md` 与 references。 | 定义 required skill mount 必须和插件包对齐的文件。 |
| `EVAL_ONLY_RUNTIME_CACHE_FORBIDDEN_PATTERNS` | eval fixture、golden、oracle、eval fast path。 | 定义绝不能进入 runtime cache 或 production resources 的评测材料。 |

Runtime cache 核准规则：

- root references 与 skill references 必须逐文件一致。
- installed plugin cache 与 workspace 目标包不一致时，只能标记 drift，不能当作可交付。
- required skill mount 与插件包不一致时，不能生成发布级质量证据。
- `sync-runtime-cache` 只能作为本地 remount aid，且必须要求 `--confirm-local-remount` 等明确确认。
- runtime cache contract 不能制造 Hub install、customer install、upgrade 或 rollback。
- eval-only 文件、oracle fast path、golden answer、rubric 不能进入 plugin cache、runtime skill 或 production resources。
- runtime cache drift 只能通过 Hub install、受控 rollback 或明确本地 remount 修复，不能通过系统 Codex 全局配置修复。

### 11.11 Sub-Agent 策略边界

Sub-agent 是执行策略，不是新的数据访问路径。只有在共享 scope pack 已通过 MCP 解析后，才允许使用 sub-agent。

适用场景：

- 全案报告涉及多个主体、账户、专题和 QA。
- 现金、支付、虚拟资产、路径、资产/债务、控制线索、事实卡和报告 QA 可以分工复核。
- 报告草稿需要第二视角或对抗式复核。

不适用场景：

- 让 sub-agent 另起事实来源、输出未验证计算或导出 raw rows。
- 单账户快速统计。
- backend 工具不可用时让 sub-agent 填补事实。
- 生成法律、税务或最终归属结论。

Sub-agent 输出必须是 bounded card：

- route。
- scope。
- tool calls。
- figures。
- warnings。
- confidence。
- deterministic fact / edge fields。
- forbidden_as_facts。
- next review action。

主 Agent 负责最终整合、claim review、报告措辞和用户可见边界，不能把 sub-agent 结果直接当最终事实。

### 11.12 Rust 边界

Rust 适合确定性、高吞吐、跨平台、窄接口计算：

- 高内存交易聚合。
- 全案 ranking。
- 去重候选族构造。
- 换卡/同事实相似度计算。
- fundgraph edge projection。
- casegraph node / edge merge。
- graph layout / projection。
- 稳定 import / cleaning 转换。
- native bridge 和离线 compute binary。

Rust 不用于：

- 为语言统一重写稳定业务逻辑。
- UI 交互和 React 展示。
- 临时 prompt 规则。
- 报告语言生成。
- 未证明性能瓶颈的普通胶水逻辑。

Rust 公共 API 必须窄：

- 输入为明确 schema。
- 输出为可复核 facts / edges / diagnostics。
- 错误为结构化状态。
- 不读写系统路径。
- 不依赖 shell-only 行为。
- macOS 与 Windows 打包路径一致可控。
- 不绕过 Evidence Ledger。
- 不返回不可解释黑盒结论。

Rust 触发矩阵：

| 触发场景 | Rust 输入 | Rust 输出 | 验收 |
| --- | --- | --- | --- |
| 高内存交易聚合 | normalized transaction schema、scope、time window、direction、dedupe policy。 | aggregate facts、rank facts、coverage diagnostics。 | 与 backend 聚合一致；错误为结构化状态。 |
| 去重候选族 | account set、holder/id scope、txn id、time、amount、balance、summary。 | same-fact family、card-replacement candidate、confidence、boundary。 | 不直接扣减 totals；候选状态进入 ledger。 |
| 资金边投影 | transaction facts、seed、direction、depth、tolerance ratio。 | supported edges、competing candidates、stop points。 | 只有 supported edge 进入 fundgraph / Mermaid。 |
| casegraph 节点/边合并 | subjects、accounts、holders、counterparties、quality gates、evidence packs。 | casegraph nodes / edges / claim support index。 | 不产生无证据边；claim support 可回溯。 |
| fundgraph layout / projection | supported edges、node attributes、scope。 | deterministic graph projection、layout hints。 | 图谱展示不改变事实边。 |
| import / cleaning deterministic transform | source schema、field mapping、cleaning rules。 | normalized facts、quality diagnostics、source warnings。 | 不绕过主系统权限和审计。 |
| native bridge / packaged binary | 明确 schema、platform target、runtime path。 | bounded compute result、structured error、health signal。 | macOS / Windows 打包路径一致，不依赖 shell-only 行为。 |

Rust 施工门禁：

- 没有性能、确定性、跨平台或内存证据时，不触发 Rust。
- Rust 只能做窄接口 compute，不生成报告语言。
- Rust 输出必须先进入 deterministic runtime、Evidence Ledger 或 diagnostic card，再进入模型可见事实卡。
- Rust 错误不能被渲染为零值事实。
- Rust binary、cache、logs 和 artifact 必须留在 Analytix-owned runtime。

## 12. 施工控制与核准机制

每轮推进必须先把目标映射到产品主线和工程分层，再动代码或文档。不能因为局部测试容易通过，就把插件改回普通事实查询入口、长 prompt、工具菜单或报告末端校验器。

### 12.0 施工入口索引

本节是后续施工的入口表。任何改动都必须先落到一个施工类型，再读取对应章节、修改对应载体、跑对应核准；不能只凭局部需求直接改 MCP、skill、prompt、README、报告模板或 eval。

| 施工类型 | 必读章节 | 必改 / 必核准载体 | 不可突破门禁 |
| --- | --- | --- | --- |
| 插件身份、manifest、界面元数据、资产 | 1.1、1.3、1.4、11.7、13.2、15.4 | `.codex-plugin/plugin.json`、`.mcp.json`、`assets/icon.png`、`assets/logo.png`、README、doctor manifest ingestion。 | 不改系统 Codex；不污染全局 home/cache/config；manifest 不写内部 intent、eval mode、assertion marker 或过程性文案。 |
| Root Skill 与渐进披露资源 | 1.2、8.7、11.10、13.5、15.4 | `skills/analytix-fund-analysis/SKILL.md`、`mcp/progressive-resources.mjs`、root/skill references 同步。 | Skill 保持短入口；生产 resources 不暴露 golden/oracle/rubric；内部 id 和过程词不进用户可见标题。 |
| 命令、路由、能力族 | 6、7.2、7.5、13.1、15.1 | `command-router.md`、`command-metadata.json`、`capability-registry.json`、`frontdoor-routing.mjs`、doctor command coverage。 | `/analytix` 稳定命名空间不漂移；命令能力族必须和 registry、metadata、tool budget、answer contract 一致。 |
| Capability Registry、Intent、Plan、预算 | 7.1、7.2、7.3、7.7、12.3 | `capability-registry.json`、`capability-registry.schema.json`、`intent-plan-protocol.mjs`、`frontdoor-answer-contract.mjs`、functional coverage contract。 | 新 capability 不能只写文档；必须派生命令、工具、门禁、doctor、functional eval、quality 维度。 |
| MCP 工具 schema 与运行时 | 3.5、7.4、7.5、8、10.6、12.4 | `tool-schemas.mjs`、`tool-input-schemas.mjs`、`tool-runtime-routing.mjs`、`tool-call-runtime.mjs`、相关 runtime module。 | 新工具不进入默认工具菜单；不能绕过 Evidence Ledger、Context Compiler、answer contract。 |
| 金额、笔数、账户、户名、对手方事实 | 2.2、3、8.1、8.2、8.3、10.3 | backend fact tools、`evidence-ledger.mjs`、`context-compiler.mjs`、diagnostic helpers、fact summaries。 | bounded rows 不得汇总成全案事实；candidate 不能升级为名下账户；schema/coverage 不明必须降级。 |
| 资金链路穿透、下一跳、回流、补调 | 4、8.2、8.4、8.5、9.2、11.12 | `fundgraph-builder.mjs`、`fund-flow-graph-runtime.mjs`、trace tools、`validate_continuation_list`、flow card renderer。 | 只有 supported transaction edge 能画 Mermaid / fundgraph；聚合排行、关键词线索、候选路径不能画成确定性交易边。 |
| 开放式假设实验室和多轮追查 | 5、7.3、8.6、10.5、15.3 | `investigation-lab-protocol.mjs`、`investigation-lab-diagnostic-runtime.mjs`、Lab card、mandatory review cards。 | 约束事实获取，不压制 Codex 提问、假设、反证和下一步追查；假设必须分级，线索不能事实化。 |
| Casegraph / Fundgraph 事实底座 | 4、8.4、13.4、14.2、15.2 | `casegraph-protocol.mjs`、`casegraph-runtime.mjs`、`fundgraph-builder.mjs`、`casegraph-roadmap.md`。 | 图谱是事实底座，不是漫游器；无证据边、无支持 claim、无 scope 的图谱不得进入报告。 |
| 报告、附件、Mermaid、Claim Review | 8.5、9、10.1、10.5、11.6 | `report-schema.md`、`claim-verifier-protocol.mjs`、`validate_report_claims`、`card-renderer.mjs`、draft/report artifact。 | 报告结论不能新增事实；unsupported claim、unsupported flow、法律越界、候选归属升级必须阻断或降级。 |
| 反模式、术语、Critique Pass | 10.1、10.2、10.3、10.5、15.2 | `anti-patterns.md`、output rule registry、card renderer、QA review、doctor synthetic risk tests。 | impeccable/Brooks Lint 机制必须覆盖普通问答、排行、图谱、Lab、报告，不只在报告末端兜底。 |
| Context、artifact、raw payload 治理 | 8.3、8.7、11.9、11.10、13.5 | `context-compiler.mjs`、`agent-output-compiler.mjs`、`agent-payload-compiler.mjs`、`mcp-artifact-store.mjs`、runtime cache contract。 | raw rows、raw graph、大 JSON、audit/detail/artifact refs 不进模型正文；artifact 不能跨案复用。 |
| Functional Eval、Doctor、发布级功能闭环 | 10.6、10.7、10.8、10.9、12.4、13.3 | `doctor.mjs`、`functional-eval.mjs` 或迁移后的功能 runner、`release-slice-audit.mjs`、`eval-coverage-contract.mjs`。 | golden/oracle/rubric 不进入 production path；doctor 不是产品质量证明；发布级功能闭环必须绑定同一包、同一 runtime、同一 skill mount。 |
| Hub、runtime cache、安装、回滚、remount | 11.7、11.8、11.10、13.3 | `hub-lifecycle.md`、`runtime-cache-contract.mjs`、`sync-runtime-cache.mjs`、`prepare-hub-package.mjs`、package hash / archive SHA256。 | runtime cache sync 只做本地同身份 remount；不能替代 Hub 安装；不能写系统 Codex home。 |
| Rust / native compute | 11.12、12.1、15.2 | Rust RFC、native binary packaging、backend bridge、cross-platform smoke。 | 没有性能、确定性、内存或跨平台证据不触发 Rust；Rust 只输出窄接口事实，不生成报告语言。 |
| 公开文案、README、默认提示、卡片标题 | 1.4、10.2、15.4 | README、manifest `defaultPrompt`、SKILL、card renderer、Lab title、report template。 | 公开文案使用商业能力语言；内部 intent、eval task、过程标签、debug/assertion 语言不能进入用户可见表达。 |

### 12.1 推进总约束

| 约束 | 内容 |
| --- | --- |
| 不改系统 Codex | 不修改、覆盖、依赖或污染系统安装的 Codex app、home、hooks、MCP、config、cache、plugin 状态。 |
| 不污染 Analytix 内嵌 Agent 默认行为 | 插件只作为 Analytix 内嵌 Agent 的可安装能力层，不改变无插件时的默认行为。 |
| 不绕过事实层 | 案件结论必须有 `case_source_envelope`、source-of-truth、当前案件 scope、计算口径、验证状态和 evidence boundary；受控 SQL/notebook 结果按 Analytix-owned Case Workbench 的交付状态进入结论。 |
| 不扩成独立业务系统 | 插件不接管 Analytix 后端、前端、案件数据平台、导出链路或人工审核流程。 |
| 不牺牲自由研判 | 约束事实获取和输出风险，不压制 Codex 的提问、假设、反证、路径选择和材料组织能力。 |
| 不堆工具 | 新工具必须先进入 Capability Registry、schema、预算、门禁、doctor、eval 和 answer contract。 |
| 不堆上下文 | raw rows、raw query result dumps、raw graph payload、大 JSON、内部 id 不能进入模型正文。 |
| 不跳过报告门禁 | 报告、附件、Mermaid、续调清单和重大 claim 必须经过对应复核。 |
| 不跨案记忆 | facts、ledger、trace、artifact、hypothesis、eval 均保持案件隔离。 |
| 不接第三方遥测 | trace、eval、artifact、ledger 留在 Analytix-owned runtime。 |

### 12.2 施工前约束

施工前必须写清：

- 本轮目标属于哪一层：Capability Registry、Command Router、Task Profile、Intent AST、Plan DAG、Deterministic Tool Runtime、Evidence Ledger、Context Compiler、Output Rule Registry、Anti-pattern Detector、Critique Pass、casegraph、fundgraph、Investigation Lab、Claim Verifier、Answer Card / Report Card、eval、doctor、runtime。
- 本轮要强化哪项产品硬能力：事实更准、追得更深、更少幻觉、更可交付、更低成本。
- 本轮不做什么，尤其是否触碰系统隔离、数据访问、默认工具面、Analytix 内嵌 Agent 默认行为、Hub 交付路径。
- 预计改动文件、最小验证命令、质量证据来源。
- 是否影响 `command-router.md`、`command-metadata.json`、`capability-registry.json`、tool schema、doctor、eval、report schema、anti-patterns。
- 是否影响普通问答、Top 排名、主体画像、资金链路穿透、开放式 Lab、报告生成、非资金 passive 边界中的任一任务族。

### 12.3 施工中约束

施工中必须保持：

- 根 reference 与 skill 内嵌 reference 同步。
- 新增 capability 同步 registry、command metadata、tool schema、doctor、eval。
- 新增事实输出同步 Evidence Ledger 和 Context Compiler。
- 新增报告输出同步 Claim Verifier、Anti-pattern Detector 和 Critique Pass。
- 新增图谱边同步 casegraph / fundgraph evidence pack 与 supported-edge 规则。
- 新增命令同步命令矩阵、路由 fallback、required gates、stop conditions 和 answer contract。
- 新增专题能力同步 domain playbook、输出准则、反模式和 report schema。
- 不把 raw rows、内部编号、SQL、artifact id、audit ref、工程黑话放进用户可见输出。
- 不因修复某个专项路径把默认 source-backed 能力层收窄成单一前门，或把低层 raw/debug 工具扩散给普通任务。
- 不因提升报告能力牺牲普通问答、排行、资金去向、Lab 和 passive 边界。

### 12.4 施工后核准

施工后必须核准：

- `node --check` 覆盖 touched JS/MJS。
- doctor、guard、health、runtime smoke 或最小相关验证通过。
- references 根目录与 skill 内嵌目录一致。
- command router、command metadata、capability registry、tool schema、doctor、eval 无漂移。
- 普通事实题工具预算为 Codex 选择一个或少数最小充分语义事实工具；报告题进入 Plan DAG 和 `validate_report_claims`，不能把报告预算套到普通研判。
- passive 非资金题不得触发资金工具。
- 没有错误金额、无证据 Mermaid、法律定性越界、候选账户写成名下账户。
- Evidence Ledger 覆盖新增 amount、count、account、holder、counterparty、edge、claim。
- Context Compiler 没有把 raw payload、大 JSON、内部 id 放入正文。
- Claim Verifier 能阻断 unsupported claim、unsupported flow、法律越界、现金/资产端过度推断。
- 质量证据覆盖运行包、manifest、MCP 工具面、references 同步状态和目标任务族。

### 12.5 偏离主线收口

出现以下情况时必须暂停扩展，进入收口审计：

- 目标无法映射到产品主线或工程分层。
- 为了提分把 golden / oracle 答案接入 production path。
- 用长 prompt 替代确定性事实层。
- 用大 JSON 替代 Context Compiler。
- 用多工具暴露替代 Capability Registry。
- 非资金题触发资金工具。
- 报告题绕过 Claim Verifier。
- Mermaid 图出现 unsupported edge。
- 质量证据不能覆盖交付运行状态。
- UI、Hub、runtime、桌面运行时排障压过资金研判主线。

收口审计动作：

```text
暂停新增功能
-> 列出改动
-> 列出目标映射
-> 列出证据缺口
-> 只修最小缺口
-> 重跑最小验证
-> 给出继续 / 停止判断
```

### 12.6 文档推进限制

- 不能删掉核心目标、硬边界、命令能力矩阵、资金链路穿透、反模式、Reference 整合边界、借鉴落点、质量门禁和推进控制层。
- 不能把专项 reference 的核心契约只留在专项文档，而不进入蓝本。
- 不能把过程记录、临时状态、单次分数、调试路径写入蓝本正文。
- 不能为了变短而删除能指导施工、核准和验收的细节。
- 能压缩的是重复解释和过程性描述，不能压缩能力契约、门禁、字段、命令、事实化风险项和质量证据。

## 13. Reference 与程序载体映射

### 13.1 Reference 整合边界

系统蓝本定义插件全局能力、边界和质量标准；专项 reference 定义某一能力域的细节；程序载体负责把这些契约落实为 runtime、doctor、eval 和输出门禁。三者必须一致。

| 文件 | 进入蓝本的内容 | 留在专项文档的内容 |
| --- | --- | --- |
| `command-router.md` | `/analytix` 稳定命名空间、命令能力族、路由 fallback、普通问答/报告/专题线索/Case Workbench/非资金任务分流。 | 命令菜单展示、触发提示、局部示例。 |
| `command-metadata.json` | 命令机器契约：lane、primary tools、required gates、stop conditions、answer contract。 | 具体 JSON schema、逐字段机器配置。 |
| `capability-registry.json` / `capability-registry.schema.json` | Capability Registry 作为 source-of-truth，派生命令、工具预算、doctor、functional eval、quality 维度。 | schema 细节、枚举值、机器校验规则。 |
| `anti-patterns.md` | P0/P1/P2 反模式、术语状态、deterministic detector、Critique Pass。 | 反模式例句、测试样例、修正文案。 |
| `runtime-boundary.md` | Data Analytics 等价来源边界、Analytix-owned runtime、artifact、auth、provenance、隐私边界。 | 运行时变量、诊断细节、操作说明。 |
| `tool-availability.md` | 默认 source-backed 能力层、完整低层工具发现受控、缺失能力降级、source/delivery blockers。 | 工具列表细节、开发诊断开关。 |
| `casegraph-roadmap.md` | casegraph / fundgraph 的节点、边、supported-edge、evidence pack、claim support、质量门。 | 路线细节、局部实现步骤。 |
| `domain-playbook.md` | 专题线索、领域术语、经侦表达边界、禁用法律表述。 | 术语短释、用户可见表达样例。 |
| `economic-investigation-analysis.md` | 公安经侦任务库、案类 typology lens、异常特征库、negative search、金额质疑包、交付包契约。 | 细化矩阵、字段清单、材料化细节。 |
| `fund-path-and-cash-bridge.md` | report-grade path、混同资金、现金桥候选、Mermaid 与事实表绑定。 | 路径输出示例、现金桥字段解释。 |
| `report-schema.md` | 报告章节、finding card、claim 状态、写作规则、结论与限制。 | 逐章字段要求和报告写作细则。 |
| `hub-lifecycle.md` | 插件分发、安装、回滚、runtime-owned 状态、交付证据边界。 | 具体操作命令、operator runbook。 |
| `plugin-benchmark.md` | OpenAI/GitHub 借鉴机制、采用项、拒绝项、非协商过滤器。 | 调研过程、排名、一次性观察。 |
| `README.md` | 对外能力概览、运行结构、命令入口、数据边界表达。 | 使用入口、展示文案、环境说明。 |
| `RELEASE_NOTES.md` | 已交付能力可用于事实核对，不作为架构依据。 | 时间线、逐次变更记录。 |
| `SKILL.md` | 短 skill、渐进披露、语义事实工具箱、`funds_investigate` navigator、Agent freedom rule。 | 触发描述和运行时加载入口。 |

### 13.2 插件包内容全景

| 内容块 | 程序载体 | 职责 | 风险边界 |
| --- | --- | --- | --- |
| 插件 manifest | `.codex-plugin/plugin.json` | 定义插件名称、界面描述、skill、MCP、资产和分发信息。 | 不能携带系统 hooks 或全局 Codex 配置。 |
| MCP manifest | `.mcp.json` | 注册 `analytix_funds` 本地 MCP server 和环境变量。 | 只能连接 Analytix-owned backend，不接第三方遥测。 |
| Root Skill | `skills/analytix-fund-analysis/SKILL.md` | 短入口、触发条件、等价边界、核心工作流、渐进披露 references。 | 不能变成长 prompt；不能教 Agent 用弱来源替代 source-backed 事实。 |
| MCP server | `mcp/server.mjs` | 组装 backend client、case runtime、资金研判引擎、工具调用 runtime、MCP request handler。 | 不能泄漏 token、raw payload、系统路径。 |
| Tool schemas | `mcp/tool-schemas.mjs` / `mcp/tool-input-schemas.mjs` | 定义语义事实工具、低层工具和输入契约。 | raw/debug/write/export/report-heavy 工具不能默认全暴露。 |
| Tool discovery policy | `mcp/tool-discovery-policy.mjs` | 控制默认 source-backed 能力层和完整发现开关。 | 默认工具面必须语义充足、低层收缩。 |
| Capability Registry | `references/capability-registry.json` | 命令、能力、工具预算、证据要求、functional eval 任务、quality 维度的事实源。 | 不能和 command metadata、doctor、functional eval 漂移。 |
| Command metadata | `references/command-metadata.json` | 机器可读命令路由、门禁、收口边界、回答契约。 | 命令不能成为工具菜单扩散。 |
| Command router | `references/command-router.md` | 人类可读命令族和研判意图映射。 | 只能作为渐进披露资料，不能默认塞上下文。 |
| Anti-patterns | `references/anti-patterns.md` | 阻断级反模式、高风险解释错误、工具扩散错误、纠偏模式。 | 不能只停留在文档，必须进入 doctor、eval、critique。 |
| Domain playbook | `references/domain-playbook.md` | 经侦资金研判术语、专题特征、禁用表述。 | 不能替代证据，不得把线索写成法律结论。 |
| Economic investigation analysis | `references/economic-investigation-analysis.md` | 公安经侦资金任务库、异常特征库、案类侦查镜头、负向搜索、金额质疑包和交付包。 | 只能启发分析和补证，不能自动法律定性或写入旧案事实。 |
| Fund path / cash bridge | `references/fund-path-and-cash-bridge.md` | 资金路径、混同资金、现金桥、输出形态。 | 不能承诺现金桥或混同资金精确归因。 |
| Runtime boundary | `references/runtime-boundary.md` | 案件事实访问、运行时隔离、auth、sub-agent、provenance 边界。 | 不能成为绕过当前案件绑定、只读 DuckDB guardrail 或 runtime 审计的例外说明。 |
| Tool availability | `references/tool-availability.md` | 默认工具、完整工具、缺失能力、source/delivery blockers。 | 不能变成 Agent 默认工具菜单。 |
| Report schema | `references/report-schema.md` | 报告章节、finding card、写作规则。 | 不能替代 Claim Verifier。 |
| Casegraph roadmap | `references/casegraph-roadmap.md` | 案件图谱节点、边、证据包和 claim support 路线。 | 不能画无证据资金边。 |
| Plugin benchmark | `references/plugin-benchmark.md` | 外部方案借鉴边界。 | 不能把外部项目照搬成系统配置污染。 |
| Blueprint execution plan | `references/blueprint-execution-plan.md` | 定义按蓝皮书审计、整改、测试、真机功能闭环和蓝皮书修正的执行基准。 | 不能替代蓝皮书；不能写入真实案件事实或发布凭证。 |
| Evidence Ledger | `mcp/evidence-ledger.mjs` | 为事实、资金边、claim、source refs 建内部证据账本。 | 用户可见输出不能暴露不透明内部 id。 |
| Context Compiler | `mcp/context-compiler.mjs` | 将工具结果压缩成模型可读事实、边界、事实化风险项、下一步。 | raw result 不进模型正文。 |
| Claim Verifier | `mcp/claim-verifier-protocol.mjs` | 报告 claim 文本风险扫描、证据锚点检查、禁用表述阻断。 | 不能只在报告末端兜底，普通研判还需要 critique。 |
| Casegraph protocol | `mcp/casegraph-protocol.mjs` | 定义主体、账户、户名、对手方、交易边、证据包、claim support。 | casegraph 是事实底座，不是漫游图。 |
| Fundgraph builder | `mcp/fundgraph-builder.mjs` | 从确定性交易 seed 构造资金流节点和边。 | 只有 supported transaction edge 才能进入图。 |
| Investigation Lab | `mcp/investigation-lab-*` | 开放式假设、证据状态、线索优先级、禁止事实化、收口边界。 | 不能把可疑特征直接升级为结论。 |
| Agent output compiler | `mcp/agent-output-compiler.mjs` | 生成紧凑、可答复、隐藏内部引用的 Agent 可读文本。 | 不能输出 raw payload 或工程调试字段。 |
| Doctor | `scripts/doctor.mjs` | 插件结构、manifest、MCP、references、ledger、claim、eval、runtime 契约检查。 | 不能只检查存在性，必须检查能力一致性。 |
| Functional eval harness | `scripts/functional-eval.mjs` 或迁移后的功能 runner / `scripts/eval-coverage-contract.mjs` | 多任务、多案件、报告复核、被动非介入、质量维度评测。 | 不能让 production path 读取 golden 答案；旧 runner 名称不得保留对比实验语义。 |
| Health / guard | `scripts/check-health.mjs` / `scripts/release-slice-audit.mjs` | runtime smoke、工具可见面、输出压缩、隔离和分发前检查。 | 不能替代真实案件质量评估。 |

### 13.3 质量脚本与证据工件映射

质量脚本是蓝本的可执行核准层。任何能力进入蓝本后，必须能被至少一种脚本、runtime smoke、eval 或人工复核卡验证。

| 脚本 | 证据类型 | 对应章节 |
| --- | --- | --- |
| `doctor.mjs` | 插件结构、manifest、MCP server、skill、references、capability、ledger、claim、eval 契约。 | 1、7、8、10、11、13、15。 |
| `check-health.mjs` | runtime/backend 健康、工具基础可用性、选案和事实链路。 | 7、8、11、15。 |
| `scripts/frontdoor-smoke.mjs` | 前门路由、Answer Card、普通问答、报告入口、passive 边界。 | 6、7、9、10。 |
| `phase4-diagnostics.mjs` | 诊断卡族、destination、rank、Lab、claim review。 | 4、5、8、9、10。 |
| `diagnostic-card-diff.mjs` | 诊断卡字段漂移和 warnings / claim_guard 丢失。 | 8、9、10。 |
| `functional-eval.mjs` 或迁移后的功能 runner | 功能闭环、事实准确、深挖、幻觉、报告、成本。 | 0、3、4、5、9、10、16。 |
| 真机功能 runner / manual UI notes | Agent UI 真实运行、工具序列、输出质量、auto-fail。 | 6、7、9、10、16。 |
| `eval-coverage-contract.mjs` | capability 任务覆盖、风险覆盖和 quality 维度覆盖。 | 6、7、10、15。 |
| `golden-qa.mjs` | 标准问答、金额、笔数、账户、户名、对手方、scope、单位。 | 3、8、9、10。 |
| `release-slice-audit.mjs` | 交付切片、manifest、MCP 身份、reference 同步、质量证据身份。 | 1、11、12、13。 |
| `runtime-cache-contract.mjs` | Analytix-owned runtime cache 和 remount 边界。 | 11、12、13。 |
| `sync-runtime-cache.mjs` | 本地 remount aid、skill mount、cache 同步。 | 11、12。 |
| `prepare-hub-package.mjs` | Hub package source、plugin package、skills、MCP、assets、references。 | 1、11、13。 |
| `dev-run-with-plugin.mjs` | 开发运行辅助和本地验证入口。 | 11、12。 |

证据工件不得只证明“脚本跑过”。它必须证明：

- 哪个 capability 被覆盖。
- 哪个命令或任务族被覆盖。
- 哪个 fact / edge / claim / output card 被覆盖。
- 是否触发 auto-fail。
- 是否验证 passive 边界。
- 是否验证 raw output、内部 id、token 和 artifact 隔离。
- 是否能回放工具序列和输出差异。

### 13.4 Reference 不可丢失清单

蓝本可以压缩重复解释，但不能丢失专项 reference 的核心契约。

| Reference | 不可丢失内容 |
| --- | --- |
| `command-router.md` | `/analytix` 稳定命名空间、registry 命令、fallback、自然语言分流、报告入口、Case Workbench 和 passive 边界。 |
| `tool-availability.md` | 默认 source-backed 能力层、完整低层工具发现受控、缺失能力降级、source/delivery blockers、resources 暴露边界。 |
| `anti-patterns.md` | blocking anti-patterns、高风险解释错误、工具扩散错误、术语状态、corrective pattern。 |
| `domain-playbook.md` | 账户/资金/交易/资产债务线索、专题词汇、法律敏感表达边界、写作措辞。 |
| `economic-investigation-analysis.md` | 公安经侦任务库、案类 typology lens、异常特征库、negative search、金额质疑包、交付包和旧线程行为产品化边界。 |
| `fund-path-and-cash-bridge.md` | report-grade path、混同资金、现金桥候选、输出形态和现金/资产端降级规则。 |
| `report-schema.md` | 报告必备章节、finding card、写作规则、claim 状态和 report gate。 |
| `casegraph-roadmap.md` | minimum nodes、edges、minimal shape、release criteria、supported-edge 和 claim support。 |
| `runtime-boundary.md` | data access、query order、performance safety、auth/runtime state、sub-agent boundary、provenance。 |
| `hub-lifecycle.md` | Hub publish/install/uninstall/rollback/remount 边界、release evidence、operator runbook。 |
| `plugin-benchmark.md` | 官方 baseline、外部项目借鉴、采用/拒绝过滤器、本地 trace/eval 边界。 |
| `capability-registry.json` | capability id、lane、commands、tool discovery contract、eval coverage contract。MCP production resource 只暴露过滤后的能力视图，eval-only 任务覆盖字段留在本地文件和 doctor/eval。 |
| `command-metadata.json` | 每个命令的 lane、主路径、门禁、收口边界、输出契约。 |
| `SKILL.md` | 短入口、渐进披露、默认前门、Agent freedom rule、硬边界。 |
| `README.md` | 对外能力概览、运行结构、命令入口、数据边界、质量证据概览。 |

整合规则：

- 专项 reference 的核心契约必须进入蓝本主章节。
- 专项 reference 的例子、调试说明、操作命令和过程记录可以留在专项文档。
- 蓝本不能引用过程状态作为架构依据。
- 蓝本和专项 reference 冲突时，以隔离边界、证据门禁、Capability Registry、Command Router、Claim Verifier 的组合判断为准。
- 修改专项 reference 后必须检查蓝本是否需要同步；修改蓝本后必须检查 root reference 和 skill 内嵌 reference 是否一致。

### 13.5 Progressive Resources 精确暴露清单

Progressive Resources 的目标是让模型按需加载插件契约，而不是默认吞入全部文档。resources 暴露必须可被 doctor 检查，且不得把 eval-only、oracle、真实案件样例、debug trace 或敏感本地状态暴露为生产资源。

允许暴露的 production resources：

| Resource | 允许原因 | 边界 |
| --- | --- | --- |
| Root Skill | 提供短入口、触发条件、硬边界和默认路线。 | 不能变成长 prompt。 |
| `command-metadata.json` | 提供机器可读命令、lane、门禁、预算和 answer contract。 | 不含 eval 答案或真实案件事实。 |
| `command-router.md` | 提供 `/analytix` 命令族和 fallback。 | 只按需加载，不默认塞上下文。 |
| `tool-availability.md` | 提供默认 source-backed 能力层、完整低层工具发现、缺失能力降级和 source/delivery blockers。 | 不作为低层工具菜单。 |
| `runtime-boundary.md` | 提供数据访问、auth、runtime state、sub-agent、provenance 边界。 | 不提供绕过当前案件绑定、只读 DuckDB guardrail 或 runtime 审计的例外路径。 |
| `hub-lifecycle.md` | 提供 publish/install/remount/rollback 边界。 | 不作为自动发布凭证。 |
| `anti-patterns.md` | 提供 blocking anti-pattern、解释错误和工具扩散错误。 | 必须进入 doctor/eval/critique。 |
| `plugin-benchmark.md` | 提供外部借鉴和拒绝项。 | 不引入外部 hook、遥测或全局配置。 |
| `casegraph-roadmap.md` | 提供 minimum nodes、edges、supported edge、claim support。 | 不允许无证据图谱漫游。 |
| `capability-registry.json` / schema | 提供 capability、工具发现契约和过滤后的能力视图。 | production resource 不暴露 `eval_coverage_contract`、`eval_tasks`、task ids、oracle 或 rubric；eval 覆盖字段只供本地 doctor/eval 读取。 |
| `top-pluginization-plan.md` | 提供 Funds 域完整参考蓝本和门禁建议。 | 不写过程记录、真实案件样例或临时状态；不取代平台 spec 或总控 authority。 |
| `blueprint-execution-plan.md` | 提供历史蓝皮书落地分解、阶段门禁和测试路径参考。 | 不作为发布质量证据，不授权 Codex Goal，不写真实案件样例，不替代当前 `AGENTS.md`、OpenSpec、平台 spec 或总控 authority。 |

禁止暴露为 production resources：

- Golden answer set。
- Golden eval rubric。
- oracle、标准答案、评测 marker、真实案件样例。
- debug trace、旧对比 output、route violation 原始记录。
- 本机路径、runtime cache 路径、token、provider config。
- release note 时间线、调试日志、临时 worktree 状态。
- raw rows、raw graph、unreviewed raw query result、artifact detail。

同步与核准规则：

- root references 与 skill references 必须镜像一致。
- resources 暴露清单必须和 `progressive-resources.mjs`、doctor required resource contract 一致。
- 新增 reference 若影响路由、事实、证据、报告、质量或边界，必须进入 resources 和本蓝本映射。
- 新增 eval-only 文件不得进入 resources。
- 修改 resource 内容后，必须重新检查 root/skill 副本、doctor、runtime cache contract 和 Hub package source。

Production resources 与 skill-local references 的区别：

| 类型 | 定义 | 例子 | 使用边界 |
| --- | --- | --- | --- |
| Production resources | MCP `resources/list` 可暴露给模型按需读取的渐进披露资源子集。 | root skill、command metadata、command router、tool availability、runtime boundary、hub lifecycle、anti-patterns、capability registry、casegraph roadmap、top blueprint。 | 只能包含生产契约和边界，不包含 eval-only 或 oracle。 |
| Skill-local references | skill package 内可由 root skill 指向的参考资料全集。 | domain playbook、fund path、report schema、plugin benchmark、专项 reference。 | 可以比 production resources 更宽，但仍不能包含 eval fixture 或真实案件样例。 |
| Eval-only fixtures | 只供 eval / assertion / doctor contract 检查的测试材料。 | golden answer set、golden rubric、oracle、quality marker。 | 不进入 production resources，不进入用户正文，不作为事实来源。 |

因此，某个 reference 没有作为 production resource 暴露，不代表它不属于插件能力；它可能已经被蓝本吸收，或作为 skill-local reference 按需加载。资源暴露策略服务于低上下文和安全边界，不是能力删减。

## 14. 外部借鉴内化矩阵

### 14.1 借鉴筛选规则

进入主线的借鉴项必须满足：

- 能强化插件包结构、隔离边界或分发闭环。
- 能减少上下文、工具调用或重复扫描。
- 能提升事实准确、证据复核、报告门禁或开放深挖质量。
- 能被 Analytix-owned runtime、doctor、functional eval 和本地 trace 约束。
- 能映射到 Capability Registry、Command Router、MCP fact layer、Evidence Ledger、Context Compiler、Claim Verifier、doctor、functional eval、Hub lifecycle 中至少一项。

明确不进入主线的内容：

- 全局 Codex hooks / config / cache 修改。
- 第三方遥测或云端 trace。
- 跨案件长期记忆。
- 无来源信封、无验证状态、无 evidence boundary 的事实路径。
- 大工具菜单、巨型 prompt、全量 JSON dump。
- 与资金研判无关的复杂方法论。

### 14.2 借鉴机制落地矩阵

| 来源 | 关键机制 | Analytix 程序落点 | 形成的插件功能 | 验收规则 | 不采用内容 |
| --- | --- | --- | --- | --- | --- |
| OpenAI Skills | Skill 是可复用工作流；通过渐进披露降低上下文。 | `SKILL.md`、`references/`、`scripts/`、`assets/`。 | 短入口 + 专项 reference + 可执行 doctor/functional eval。 | root skill 保持短；reference 一层可达；普通问答不加载全量文档。 | 巨型 skill、把所有领域知识塞入默认 prompt。 |
| OpenAI Build Plugins / openai/plugins | Plugin 是稳定分发单元，组合 skill、MCP、assets 和 manifest。 | `.codex-plugin/plugin.json`、`.mcp.json`、plugin package boundary、Hub lifecycle。 | 插件以完整包交付，不靠用户手工复制 prompt。 | doctor 检查 manifest、skills、MCP、assets、required files。 | 把 eval fixture、临时脚本、系统路径放入 production resource。 |
| MCP Tools 规范 | 工具有名称、schema、结构化结果、安全边界和工具调用可见性。 | tool schemas、output policy、runtime boundary。 | 工具有输入契约、输出压缩、错误状态和安全边界。 | schema 可加载；错误不被写成零值事实；raw payload 不进用户正文。 | 无 schema 工具、裸结果倾倒、模型侧自行解析全量 payload。 |
| Data Analytics（母版基准） | index 只路由、轻量 user-context preflight、required/optional source access、source-of-truth selection、semantic layer as map、focused workflow、live source verification、SQL/Python/notebook 正常可用、data validation quality bar、report/visual/dashboard/spreadsheet/notebook delivery contract、风险触发升级和低上下文默认。 | root/index、`case-context`、focused skills、`visual-evidence`、`case-workbench`、`report-builder`、`data-quality`、`claim-review`、artifact/attachment contracts、性能/上下文预算、Evidence Ledger。 | Analytix 以 Data Analytics 插件形态交付：对象画像、全案分析、报告、证据表格、Top20 图表、看板卡、附件工作包和受控 Workbench 各有 owner、source、validation、delivery 和 completion gate；语义工具不足时能自然升级 SQL/notebook 并回填 evidence。 | 每个交付面有 Use When/Do Not Use/Workflow/Completion Gate；普通问题轻量、风险问题升级；图表和表格必须证据绑定；notebook/SQL 必须 current-case/read-only/cleaned-scope/auditable；同一 claim 多来源时必须 source-of-truth selection；没有真实 artifact 不得说已交付。 | market sizing、KPI/business metric 语义、依赖 Data Analytics runtime、把 Jupyter 作为普通用户默认路径、把 Data Analytics 的行业语义照搬到经侦案件。 |
| Investment Banking | invocation gate + lead skill handoff + internal support；router 不做实质业务、不决定最终展示面；lead workflow 拥有第一层真实判断、deliverable intake、artifact hierarchy 和 final response。 | root/index/`funds_investigate` 降为入口与 navigator；`pair-amount-investigation`、`quick-fact`、`subject-dossier`、`fund-tracing`、`full-case-analysis`、`visual-evidence`、`report-builder` 作为经侦 lead workflow owner。 | 金额核验、主体画像、资金穿透、全案报告、图表/附件分别有 owner，support MCP/card/workbench 只供证据、验证和 artifact；用户先看到可办案的 hero deliverable。 | 用户不看到 routing、handoff JSON、support 字段、case_id、workflow、debug label；support 层不得写最终答案草稿；最终交付像办案材料。 | 投行业务语义、估值/融资/交易流程内容。 |
| Public Equity Investing | final deliverable framework + support-layer hidden + source posture near claims + professional judgment heuristics；human-readable artifact first，support/audit files 留幕后。 | `investigative_judgment_layer`、`professional_judgment_contract`、`support_layer_hidden_contract`、`hero_delivery_contract`、`evidence_maturity_language_contract`。 | 每个实质回答把已验证事实转为经侦判断：资金意义、异常特征、关系/角色线索、证据缺口、暂不能认定事项和下一步证明路径。 | 事实正确但只是数字/口径/边界摘要判失败；必须回答“这笔钱意味着什么、哪里异常、还缺什么、下一步查什么”；重要 claim 附近必须交代证据成熟度。 | 投资建议、评级、目标价、交易动作等证券业务语义。 |
| Data Analytics + IB + PE 专业成稿骨架 | report spine、memo plan、evidence posture、decision hinge、PM judgment 共同保证专业读者可用。 | `案件研判 spine`、`研判材料 plan`、`证据成熟度`、`侦查判断点`、`delivery-qc`。 | 普通答案、金额核验、主体画像、流向图、全案报告都从事实转成经侦材料：结论、事实表、异常、意义、缺口、动作。 | 只给数字/口径/工具说明/审计标题判失败；必须像公安经侦材料。 | 业务行业术语、投资/交易建议、金融建模结论。 |
| Skill Creator（Codex/Anthropic） | `description` 是触发主入口；skill 三层渐进披露：metadata 常驻、`SKILL.md` 按触发加载、references/scripts/assets 按需加载；确定性/重复动作进入 scripts；用真实任务 forward-test 和保留样本避免过拟合；skill 内容不能让用户意外。 | 所有 focused skill 按“短 SKILL + 明确触发 + Use/Do Not Use + workflow + completion gate + 一跳 reference”重构；经侦术语、案例方法、表格/报告样式放 references；schema/profile/导出/渲染/泄漏检查等机械质检进入 scripts。 | skill 不再堆成长 prompt 或说明书；用户问金额、画像、流向、报告时能触发正确 owner，support 层只给证据，最终由 owner 成稿；复杂专题读取对应 reference，而不是默认加载全蓝本。 | `SKILL.md` 原则上低于 500 行；frontmatter description 覆盖真实触发语且不过度泛化；reference 直连且按需读取；forward-test 覆盖简单/复杂/多轮/旧线程模式；失败要泛化修复，不做单案硬编码；测试答案和真实案件事实不得进入 production skill。 | 把 eval/golden/测试答案写进 production skill；为了触发率过度扩大 skill；把 skill 写成硬路由、硬停止、长模板或替代 MCP/source verification 的口号。 |
| `crystaldba/postgres-mcp` 数据库现场诊断机制 | tool-only API、schema/object inspection、restricted/unrestricted access、AST-backed safe SQL、forced read-only execution、timeout、EXPLAIN、top-query/workload analysis、database health、deterministic index-analysis algorithms。 | `case-workbench`、`run_case_sql`、`explain_case_sql`、`diagnose_case_sql`、`profile_case_schema`、`preview_case_rows`、`inspect_workbench_history`、`case_sql_recipes`、`count_case_rows`、DuckDB EXPLAIN parser/binder guardrail、query-log/slow-path diagnostics、materialized-index/graph health oracle、Evidence Ledger、`agent-payload-compiler`。 | Analytix 可像数据库现场勘查一样先看表结构、字段语义、字段覆盖、查询计划、失败原因、受控样本、历史查询和经审核 recipe，再决定 MCP 语义工具、SQL、Python 或 notebook；对慢查询、图谱重建、索引缺口和派生表断裂给出确定性诊断；SQL 是可审计 source-backed 能力，不是绕过插件。 | DDL/DML、transaction control、COPY/EXPORT/ATTACH/INSTALL/LOAD、raw 表、外部文件、网络、secret、跨案 join、无限 `SELECT *` 默认阻断；只读查询必须带 purpose、限行、当前案件、清洗/analysis scope、DuckDB parser/binder 预检和 evidence 回填；结构化解析是主边界，字符串扫描只能辅助；健康/计划/样本不得升级为案件金额或资金路径事实。 | 暴露直连数据库凭据、启用 unrestricted SQL、安装数据库扩展、照搬 Postgres-only health checks、让 DB MCP 产最终中文结论、把 read-only bypass 当唯一安全边界、把样本/计划当金额 claim、让模型自行扫描文件系统找数据库。 |
| 无插件旧线程成功模式 | Codex 能理解 Analytix 软件工作流，在多轮中切换清洗表、analysis 索引、图谱、报告、附件和全文核算。 | `analytix_workflow_context`、`case-context`、`case-workbench`、`visual-evidence`、`report-builder`、`delivery-qc`。 | 让 Codex 更懂 Analytix：当前案件、清洗导出、明细附件、报告续写、图谱导出、多轮继续推进都成为插件能力。 | 用户要明细/图/报告/继续深挖时，插件必须沿 Analytix 工作流推进，不输出固定事实卡。 | 把旧案件姓名、金额、固定结论写入 production reference。 |
| codegraph | 预索引图谱，减少反复扫描、token 和工具调用。 | casegraph protocol、casegraph runtime、fundgraph builder。 | casegraph 预压缩主体、账户、对手、质量门、证据包。 | casegraph 能支撑 scope/rank/trace/claim-support。 | 图谱漫游、无证据边、把聚合排行当交易链。 |
| context-mode | raw tool output 留在隔离层，只把压缩结果给模型。 | context compiler、artifact store、output policy、`agent-thread-audit.mjs`。 | 模型只看研判事实摘录；raw rows、raw graph、support envelope 留在本地 artifact / `_meta` / ledger。 | 输出长度、内部 id 扫描、structuredContent/debug payload 受控；真实 rollout 审计统计 tool calls、MCP payload bytes、模型可见 JSON/重复 JSON/internal leak。 | 大 JSON 或转义 JSON 直接给模型、把 artifact id/support 字段当用户证据。 |
| impeccable | 专业词汇、命令体系、反模式规则、deterministic detector、LLM critique pass。 | anti-patterns、Output Rule Registry、Critique Pass、command metadata。 | 建立 ask/qa/rank/lab/flowgraph/report/evidence/review 输出准则。 | doctor/functional eval 覆盖反模式；普通问答、图谱、Lab、报告均做 critique。 | 只借命令名、不落地 detector；只在报告末端审查。 |
| Brooks Lint | 规则化 lint 与风险质量门。 | anti-pattern rules、guard、quality dimensions。 | 对无来源强答、弱来源冒充、bounded-row totals、候选归属升级、单位漂移、unsupported flow 做确定性拦截。 | blocking anti-pattern 为 auto-fail。 | 主观“看起来不错”替代规则。 |
| codex-plugin-cc | read-only review、adversarial review、status/result/cancel。 | Claim Verifier、report review、后台分析状态、对抗式复核 lane。 | 报告 claim review、unsupported flow review、法律越界 review。 | write_blocked 可解释。 | 跨 agent hook 行为、让外部 agent 接管案件事实。 |
| superpowers / Aegis | 工作流纪律、baseline-first、evidence-verified、drift-check。 | `/analytix plan`、Task Profile、Plan DAG、guard。 | 复杂任务先给 lane、预算、门禁和收口边界。 | 预算、漂移、证据完整性进入 functional eval。 | 复杂方法论覆盖资金研判主线。 |
| oh-my-codex | 不替代 Codex，只增加 workflow、state、doctor。 | Analytix-owned workflow state、doctor、runtime health、case journal。 | 插件增强 Analytix 内嵌 Agent，不改系统 Codex。 | 隔离门禁通过；不写全局 config/hooks/cache。 | 全局安装污染、scheduler/hook 接管。 |
| AgentOps / Langfuse | trace、eval、反馈回放。 | 本地 trace、真机功能 artifact、functional replay。 | 能回放工具序列、route violation、输出长度、质量信号和 auto-fail 原因。 | trace 本地保存、脱敏、案件隔离。 | 第三方遥测、跨案行为画像。 |
| agentmemory | 记忆生命周期、检索、回放、治理。 | 案件隔离 artifact、短期研判状态、case journal 生命周期。 | 多轮追查可复用本案 scope、hypothesis、evidence。 | case_id / scope 绑定；过期和清理策略明确。 | 通用长期记忆、跨案事实复用。 |
| wshobson/agents | source-of-truth 生成多平台工件、评测和治理。 | `capability-registry.json`、command metadata、tool schema、doctor/functional eval 派生。 | Capability Registry 成为命令、工具预算、门禁、quality、functional eval 的上游事实源。 | registry 与 command metadata/tool schema/functional eval 无漂移。 | 扩成多平台泛插件市场。 |
| compound-engineering-plugin | strategy -> plan -> work -> review -> compound 闭环。 | 研判计划、事实执行、claim review、经验沉淀。 | 案件研判 -> 证据复核 -> 报告交付 -> 本案经验卡闭环。 | 经验只能沉淀为规则/反模式/functional eval，不写跨案事实。 | 让经验沉淀污染案件事实。 |

#### 14.2.1 P1 修复后逐项核验矩阵

本矩阵记录本轮必须达到的成熟插件等价机制和经侦增强。真实案件姓名、金额、明细只保存在本地功能闭环输出，不写入蓝本正文；普通办案用户只应看到经侦业务语言和交付成果。发布证据必须包含 `P1 env smoke outputs`：真实案件 env-driven P1 smoke 按 Pass / Partial / Fail 记录任务质量、事实正确、经侦表达和交付物可用性，不得用 full_plugin 分数或 direct MCP 正确性替代真实桌面端判断。

2026-07-01 复核结论：本机已重新读取 Data Analytics、Investment Banking、Public Equity Investing 的已安装插件源码，并通过 OpenAI Codex manual 与 Anthropic Agent Skills 官方资料校准 Skill Creator 机制。本轮新增 `scripts/skill-creator-alignment-audit.mjs`，把“短 skill + 明确 description + 一跳 references + deterministic scripts + real-task forward-test + production skill 不含 golden/固定案件事实”的要求转成可重复跑的 L0 gate；`doctor` 已纳入该脚本语法检查。0.16.9 的 direct MCP DuckDB oracle、real front-door oracle A-G、L5 review 和 artifact inspection 仍是事实正确性基线；后续 skill/reference 修改还必须补跑 skill creator alignment，不能只用旧的 6 月 P1 文档判断成熟插件等价。

2026-06-11 复核结论：2026-06-10 的 P0 收口结论只能作为阶段性修复记录，不能继续视为商业闭环完成。019eafd0 后续真机问答证明，最终回答仍可能停留在 fact-card 摘要和统计范围解释，缺少经侦判断层、大额交易表、异常特征、资金意义和下一步侦查动作。宿主 Codex UI chrome 里的过程卡片仍只记录为 host UI limitation；插件 blocker 只看最终回答、MCP/card 返回、图谱、表格、报告、附件标题、事实/口径/交付质量和专业判断层是否达标。

| 借鉴项 | mature principle | Analytix 落点 | parity/gap | 经侦增强 | 证据文件 | release blocker |
| --- | --- | --- | --- | --- | --- | --- |
| Data Analytics：多 skill / source guardrail / 核验状态 / 交付要求 | index 路由、来源预检、focused workflow、实时来源读取、核验状态、交付要求共同构成闭环；SQL/Python/notebook/DuckDB/shell 是受当前案件和只读口径约束的可复核路径。 | `skills/index/SKILL.md`、`references/focused-skill-shared.md`、`mcp/frontdoor-runtime.mjs`、`mcp/frontdoor-answer-contract.mjs`、`mcp/agent-output-compiler.mjs`、`mcp/agent-payload-compiler.mjs`。 | Partial：source-backed 路径已有基础，但真实前门回答仍没有稳定形成专业经侦交付。 | 公安经侦材料表达：结论先行、事实表格、研判意义、证据边界短写、补证建议；内部 source/validation 留 `_meta`/ledger。 | `user_delivery_contract`、`user-visible-language-smoke.mjs`、`frontdoor-smoke-real-*`、新增 `professional_judgment_contract`。 | Yes：发布前必须通过真实多轮专业交付验收。 |
| Data Analytics：user-context / source envelope | 预检只做来源路由信封，不替模型给结论；semantic layer 只是候选地图。 | `case_source_envelope`、`skills/case-workbench/SKILL.md`、`references/focused-skill-shared.md`、`mcp/agent-context-hygiene.mjs`、`mcp/frontdoor-runtime.mjs`。 | Pass：当前案件、事实来源、清洗/analysis scope、质量、核验、交付、缺口均有 contract。 | 当前案件隔离、摘要缺失、重复/换卡风险、资金口径边界、补调建议进入经侦口径。 | `case_source_envelope_contract`、`case_workbench_control_contract`、`output/analytix-fund-analysis/functional-closure/2026-06-10T06-39-04-972Z/functional-eval.json`。 | No。 |
| Data Analytics：validate-data / critique | 来源、方法、计算、图表、结论和未验证项都要复核；报告渲染通过不等于事实通过。 | `skills/claim-review/SKILL.md`、`skills/analysis-critique/SKILL.md`、`mcp/user-facing-language.mjs`、`mcp/card-renderer.mjs`、`mcp/agent-output-compiler.mjs`。 | Partial：内部词和机械标题治理不足以证明判断质量；需新增专业判断层 critique。 | 用户交付 critique 覆盖机械口吻、无研判意义、无表格、无资金意义、无下一步核查。 | `user-visible-language-smoke.mjs`、`functional-eval.json`、`professional_judgment_contract`、B1/B2 frontdoor smoke。 | Yes：fact-card 摘要化必须被阻断。 |
| Data Analytics：visualize-data / build-report | 图表和报告必须有真实交付面、来源 metadata、最终上下文 QA；不得用聊天摘要冒充附件。 | `skills/visual-evidence/SKILL.md`、`skills/report-builder/SKILL.md`、`references/report-schema.md`、`mcp/fund-flow-graph-runtime.mjs`、`mcp/fundgraph-builder.mjs`、`mcp/card-renderer.mjs`。 | Partial：图谱/报告已有基础，但真实反馈仍显示流向图不穿透、主体研判不专业、标题像审计清单。 | 图谱改为资金来源、主要链路、下游去向、资金断点、待补证事项；报告/表格围绕研判结论。 | `frontdoor-smoke-real-*`、新增 `hero_delivery_contract`。 | Yes。 |
| Investment Banking：lead workflow / support hidden | router 只做 admission 和 lead skill selection；lead workflow 负责第一层真实判断和 hero artifact；support 不外露。 | root/index/`funds_investigate` 降为 navigator；focused skills 成为金额、主体、流向、全案、图表/报告 owner。 | Partial：navigator 方向已有，但 support/card 仍可能诱导最终答复，lead owner 边界未闭合。 | 经侦 lead workflow 产出办案材料，而不是工具解释。 | lead-owner 静态扫描、support-layer hidden 测试、真机最终回答审查。 | Yes。 |
| Public Equity Investing：专业判断 / hero artifact first | 用户先看 human-readable artifact；support/audit files 隐藏；实质输出回答 so what、风险、证据缺口和行动。 | `investigative_judgment_layer`、`professional_judgment_contract`、`support_layer_hidden_contract`、`pair_amount_mini_investigation_smoke`。 | Partial：当前金额回答仍只有口径对照，缺少异常扫描、关系/资金目的研判和下一步侦查路径。 | 转化为经侦判断：资金性质、角色关系、异常特征、下游去向、补证动作。 | Pair Amount、主体画像、资金流向图、全案报告多轮真机验收。 | Yes。 |
| Impeccable | 领域术语、反模式 detector、critique 覆盖普通问答、Lab、图谱、报告。 | `skills/quick-fact/SKILL.md`、`skills/case-workbench/SKILL.md`、`skills/fund-tracing/SKILL.md`、`skills/visual-evidence/SKILL.md`、`skills/report-builder/SKILL.md`、`skills/claim-review/SKILL.md`、`skills/analysis-critique/SKILL.md`、`mcp/user-facing-language.mjs`。 | Partial：经侦术语替换已有，但反模式还必须覆盖“事实正确但没有专业判断”的低质输出。 | 经侦交付反模式：内部标题外露、无表格、无异常研判、无资金意义、无下一步核查均失败。 | `user_delivery_contract`、`output_leak_contract`、`professional_judgment_contract`、`frontdoor-smoke-real-*` P1 输出。 | Yes。 |
| CodeGraph | casegraph/fundgraph 只给上下文和证据索引，不替模型决策；候选边不得自动升级为确定路径。 | `mcp/fundgraph-builder.mjs`、`mcp/fund-flow-graph-runtime.mjs`、`mcp/frontdoor-runtime.mjs`、casegraph/fundgraph references。 | Pass：casegraph/fundgraph 仍只供上下文和证据索引，用户图谱表达转为主链、旁路线索、未调取端点和资金断点。 | 资金主链、旁路线索、现金/理财/证券/基金去向以经侦图谱语言和可读图/表呈现。 | `frontdoor-smoke-real-2026-06-10T15-35-55-138Z.json`、`frontdoor-smoke-real-2026-06-10T15-53-58-258Z.json`。 | No。 |
| Superpowers / Aegis | 复杂全案/报告/重大 QA 才计划、复核、分工；简单快查保持最小充分路径。 | `references/runtime-boundary.md`、`references/top-pluginization-plan.md`、`skills/report-builder/SKILL.md`、`skills/analysis-critique/SKILL.md`、`mcp/frontdoor-runtime.mjs`。 | Pass：策略和边界已落地，P1 快查未被重型流程拖慢；复杂全案首轮先给待复核材料，正式报告分工仅作内部复核。 | 分工只服务内部分析复核，用户只见经侦结论、图表、附件、补证建议。 | `runtime-boundary.md`、P1 quick-fact smoke、full-case/report smoke、B1/B2 smoke。 | Conditional：P1 不阻断；复杂正式报告需保留内部复核记录。 |
| oh-my-codex | 只借 doctor/runtime 状态治理，不替代 Codex，不污染系统配置。 | Analytix-owned doctor、runtime cache、case journal、Hub package、`scripts/prepare-hub-package.mjs`、`scripts/functional-eval.mjs`。 | Pass：本轮改动限定在插件目录，package isolation 和泄漏检查保持 Analytix-owned 边界。 | 插件运行环境普通用户默认不显示，隔离证据留在开发/发布材料。 | `hub_package_isolation_contract`、`runtime-boundary.md`、`output_leak_contract`。 | No。 |

#### 14.2.2 子任务分工规划

- 复杂全案报告、重大金额争议、资金穿透多层追查、图像/报告 QA 可以拆分为事实核算、路径复核、图表质检、材料措辞复核。
- 所有分工共用同一当前案件、同一数据来源与口径边界、同一核验依据；不得另起事实来源或越过只读、清洗表、analysis scope。
- 主 Agent 负责最终研判结论、证据边界、补证建议和经侦材料口吻；普通用户不得看到内部任务分工、review pass、workflow、工具过程或调试标签。

#### 14.2.3 顶级经侦插件落地任务包

本节把 14.2 的成熟插件借鉴收敛成施工时必须落地的能力包。任何后续改造不能只在文案里声明“对齐 Data Analytics / Investment Banking / Public Equity Investing”，必须能在 skill、MCP、runtime、doctor、functional eval 或真实 UI 闭环中看到对应能力。

| 能力包 | 必须落地 | 验收口径 |
| --- | --- | --- |
| Source preflight / source-of-truth | `case_source_envelope` 输出当前案件、清洗/analysis 覆盖、必要来源、可选 enrichment、质量缺口和交付要求；同一 claim 多来源时做 source-of-truth selection。 | case-project source 不明 fail-fast；MCP、Workbench、报告和图表不能各用一套口径；差异无法解释时输出 Claim Gap，不取最大值或最新值。 |
| Semantic layer as map | schema、casegraph、fundgraph、rank、profile、scope map 只给候选对象、字段、路径、风险和 query guidance。 | 语义层结果没有 live verification 时不得写成报告级确定事实；rank/sample/preview 不控制金额 claim。 |
| Live verification / Workbench | MCP 事实层优先，语义事实不足、冲突、被挑战或需要自定义口径时进入受控 SQL/Python/notebook。 | SQL/notebook 结果必须 current-case/read-only/cleaned-scope/purpose/limit/auditable，并回填 Evidence Ledger；不得把 SQL 当禁用项或默认首跳。 |
| 经侦 lead owner | 金额核验、主体画像、资金穿透、全案、图表、报告由 focused owner 成稿；support 层只供证据和验证。 | 用户最终看到结论、事实表、异常、资金意义、证据成熟度和补证动作；看不到 routing、support JSON、workflow、case_id 或工具说明。 |
| 续调后置门槛 | 当前案件 DuckDB 可得数据在用户问题范围内已分析充分，且下一层资金穿透依赖外部材料时，才给续调建议。 | 续调清单必须列明对象、账号/户名、期间、材料、目的和当前数据断点；泛化建议判失败。 |
| Skill Creator 结构化 | focused skill 保持短入口，frontmatter description 管触发，一跳 reference 管领域知识，scripts 管确定性检查。 | `SKILL.md` 不膨胀成蓝本；真实任务 forward-test 覆盖触发、成稿、拒绝错误结论和多轮继续追查；生产 skill 不含测试答案或真实案件事实。 |
| DB MCP 诊断增强 | `profile_case_schema`、`preview_case_rows`、`explain_case_sql`、`diagnose_case_sql`、`inspect_workbench_history`、`case_sql_recipes` 和 DuckDB EXPLAIN parser/binder guardrail 进入 Workbench 路线，结果统一写入 evidence/provenance 并由 payload compiler 压缩。 | 能看表结构、字段覆盖、查询计划、失败原因、受控样本和可回放 recipe，但不能绕过当前案件、只读、清洗/analysis、Evidence Ledger、source-of-truth selection 和用户可见语言边界。 |

## 15. 能力契约与落地验收映射

本章只定义插件应具备的完整能力契约和验收口径，不按实现状态分层。后续施工、核准、审计和补齐都以本章为能力对照面。

### 15.1 能力契约矩阵

| 能力面 | 程序载体 | 验收口径 |
| --- | --- | --- |
| 插件包身份 | `.codex-plugin/plugin.json`、`.mcp.json`、短 `SKILL.md`、root/skill references、MCP server、scripts、assets。 | 质量证据必须绑定交付运行状态、plugin package、runtime、skill mount。 |
| 默认工具面 | 默认 `tools/list` 暴露可审计语义事实工具箱：scope、rank、profile、trace、casegraph/hypothesis、quality、validation 与 `funds_investigate` navigator。 | 低层 raw/debug/write/export/report-heavy 工具不得默认暴露；新增语义工具先进入 registry、metadata、doctor、eval。 |
| 自然语言导航 | `frontdoor-runtime.mjs` / `frontdoor-routing.mjs` 承接 Task Profile、Intent AST、route hint 和轻量事实卡。 | `funds_investigate` 不得成为总指挥；一跳金额、Top 排名、主体画像、开放深挖、资金穿透、非资金 passive 必须允许 Codex 直接选择对应语义事实工具。 |
| 案件范围与 casegraph | `get_case_scope_map`、`casegraph-runtime.mjs`、`casegraph-protocol.mjs` 承载范围、质量门、Top 实体、证据包和 claim support。 | casegraph 必须从局部事实卡推进为可查询、可复核、可预索引的事实底座。 |
| 排名与主体画像 | account/holder/counterparty rank、holder analysis、owner/account/holder scope。 | Top 必须绑定 metric、scope、direction、time window、success filter、coverage；候选不能升级为归属事实。 |
| 资金链路穿透 | destination/outflows/path/flowgraph、`trace_subject_top_outflows`、`trace_fund_next_hop`、`trace_fund`、`fundgraph-builder`。 | 多跳 balance feasibility、amount gap、time gap、competing candidates、visible-data cut-off、补调闭环必须硬化。 |
| Investigation Lab | `run_investigation_lab`、`hypothesis_probe`、现金/理财/专题探针。 | 每张高价值卡必须有 deterministic follow-up、证据缺口、收口边界和 forbidden-as-fact。 |
| Evidence Ledger | `evidence-ledger.mjs` 覆盖 amount、count、account、holder、counterparty、flow edge、report claim、source refs。 | Ledger coverage 必须覆盖所有用户可引用事实，尤其是 Mermaid、补调清单和报告 finding。 |
| Context Compiler | `context-compiler.mjs`、output compiler、artifact store。 | raw rows、raw graph、大 JSON、audit/detail/artifact refs 不得进入模型正文。 |
| Claim Verifier | `claim-verifier-protocol.mjs` 阻断金额 claim、unsupported flow、法律敏感表述、候选归属、现金/资产过度推断。 | 普通问答、排行、图谱、Lab 还需要 Anti-pattern Detector 和 Critique Pass。 |
| 报告交付 | `/analytix`、`/analytix report`、`run_full_case_analysis`、`report-schema.md`。 | 报告链路必须绑定 supported edge + evidence table；`write_blocked` 时只能给待复核草稿和补证动作。 |
| 质量闭环 | doctor、functional eval、真机功能验收、guard。 | 功能闭环必须覆盖事实准确、追查深度、幻觉抑制、报告质量、工具成本、被动边界。 |

### 15.2 能力校准与隐患边界

| 校准点 | 风险 | 校准结论 |
| --- | --- | --- |
| 插件定位不能越界 | 插件被写成独立业务系统或接管 Analytix 主系统。 | 插件身份是 Skill + MCP + references + scripts + assets 的组合包，事实归 analytix data/runtime backend / MCP 事实层。 |
| 单 root skill 不能长期代表商业成熟形态 | 插件页只显示一个 skill，用户看不出对象研判、全案分析、资金穿透、报告复核等 focused workflow。 | MVP 可保留兼容 root skill；商业成熟验收必须有 Data Analytics 式 focused skill 产品面，并由 command metadata、resources、插件界面能力描述和命令矩阵共同支撑。 |
| 当前案件同步必须产品化 | UI 已选案件但 current case-project binding 为空，Agent 要求用户再次提供 `case_id`。 | UI -> current case-project binding -> MCP runtime 必须自动同步；同步失败是运行时边界，不是让用户重复确认的常态流程。 |
| 命令路由不能缺位 | 只盯底层工具和事实卡，忘记 `/analytix` 命名空间、fallback、passive 边界。 | command router、command metadata、capability registry 必须一致；命令族不能扩成默认工具菜单。 |
| 追踪类“全面”不能误入报告门禁 | 多主体资金梳理、关联通道、资金追踪被当成全案报告，输出 `write_blocked` 模板而非结果。 | 非报告追踪意图进入 investigation-lab / subject-outflow-tracing；正式报告意图才进入 report lane。 |
| 输出准则必须成为一等能力 | 报告 claim 能阻断，但普通问答、排行、图谱、Lab 缺 critique。 | impeccable 式反模式、术语状态、Critique Pass 必须全链覆盖。 |
| 固定 golden 不能泄漏生产输出 | 运行时默认主体、固定交易号、固定金额或固定时间集合污染其他案件。 | 固定样例只能存在 eval fixture 或样例任务；production runtime 只能从用户问题、当前案件和事实层生成主体/金额/交易号。 |
| 外部借鉴必须分层取舍 | 把完整框架、全局 hook、遥测、长期记忆、工具菜单全部写进 Analytix。 | 只吸收能映射到 registry、MCP fact layer、ledger、compiler、verifier、doctor、eval 的机制。 |
| 缺口不能被文档覆盖 | 蓝本写完整被误当能力已成熟。 | 每项能力都必须能落到 doctor、eval、runtime health、claim review 或专项测试。 |
| 领域知识不能只留在专项文档 | 只做事实查询和 claim 门禁，丢失资金研判味道。 | domain playbook 和 fund path 规则必须进入蓝本主定义。 |
| 工具面过度收缩或过度扩散 | 过度收缩会让 Codex 只能机械走前门；过度扩散会造成成本、口径和 raw payload 风险。 | 默认暴露语义事实工具箱，隐藏低层 raw/debug/write/export/report-heavy 工具，新增工具先进入 capability registry。 |
| 资金链路穿透能力必须坐实 | 有穿透工具但报告级链路不稳定。 | trace、fundgraph、Mermaid、补调清单、Claim Verifier 必须统一进 Ledger 和 Context Compiler。 |
| Claim Verifier 不能单独承担全部风险 | 报告复核覆盖不到普通研判幻觉。 | 金额问答、Top、主体画像、Mermaid、开放假设都需要 Anti-pattern Detector 与 Critique Pass。 |
| casegraph / fundgraph 是质级提升核心 | 图谱停留在路线图或局部图谱，插件回到多工具查询。 | 它们必须成为预索引事实底座，支撑低 token、多轮追查和证据回溯。 |
| 功能闭环是发布前真实验收，不是追分主线 | 施工线程围绕脚本分数反复跑，忽略真实用户问题、runtime cache 和当前案件数据。 | 施工阶段先修能力、契约、工具和输出；发布前 functional closure 证明真实 Analytix 内嵌 Agent + 当前插件 + 当前案件数据下 source + validation + delivery 闭环成立。 |
| Health pass 不能替代真实任务质量 | 固定 golden 被跳过、deep health 全绿，但真实用户任务失败。 | release-quality 必须包含真实语义工具 / navigator 任务、真机功能验收、runtime cache preflight 和用户可见结果检查；skipped golden 只能作为运行健康证据。 |
| Rust 应以性能和确定性证据触发 | Rust 变成泛化重写。 | 只用于交易聚合、图谱构建、去重候选、高内存转换、跨平台 native compute 等窄边界。 |

### 15.3 自主研判保留条款

插件的约束对象是事实获取、证据表达、数据访问、报告 claim 和交付门禁，不是 Codex 的侦查思考能力。蓝本必须防止两种退化：一种是放任模型把无来源、弱来源或未验证计算写成事实，另一种是把 Codex 压成只能读事实卡的一问一答工具。Controlled Case Workbench 正是两者之间的产品化通道：允许自定义计算，但把数据、权限、口径、行数和证据状态留在 Analytix 边界内。

必须保留的 Codex 自主能力：

- 理解用户真实侦查意图，而不是只匹配命令字面。
- 提出开放式假设，包括金额集中、时间集中、换卡、现金断点、理财、资产、支付通道、缺失对手方和控制/代持线索。
- 主动质疑 scope、coverage、source、清洗、重复、单位和报告口径。
- 比较不同口径下的事实差异，并指出哪一种口径可交付。
- 根据证据缺口设计下一步追查、补调材料和人工确认动作。
- 对报告草稿、Mermaid、附件和结论做对抗式复核。
- 在证据不足时主动降级、阻断或请求用户确认。

不能被“约束”误伤的场景：

- `answer_card_complete=true` 只表示当前问题可以回答，不表示用户不能提出新假设、新对象、新时间窗或新路径。
- `max_additional_tools=0` 只限制同一问题的无效追加调用，不限制下一轮新任务。
- 默认 source-backed 能力层用于扩展 Codex 的可用认知器官；只限制低层 raw/debug/write/export/report-heavy 工具，不限制 Codex 思考、计划和工具组合。
- Claim Verifier 阻断正式报告措辞，不阻断列出线索、缺口、反证和补证动作。
- passive 非资金边界只防止误触发案件工具，不影响普通 Codex 完整回答非资金任务。

自由研判的合格形态：

```text
自由提出问题
-> 明确假设状态
-> 进入确定性事实层
-> 编译证据卡
-> 复核 claim 和边界
-> 输出事实 / 线索 / 缺口 / 下一步
```

### 15.4 公开文案与内部 Intent 分离

内部 intent、eval task id、diagnostic label 可以保留工程含义，但不能直接进入公开文案、manifest 展示、默认提示、用户可见标题或正式报告措辞。公开文案必须表达商业产品能力，不表达历史过程、调试来源或评测来源。

内部到公开的命名映射：

| 内部名称 | 内部含义 | 公开表达 |
| --- | --- | --- |
| `old_thread_patterns` | 侦查式不规律模式深挖 lane。 | 开放式深挖、侦查式线索发现、不规律模式研判。 |
| `functional_replay` | 功能回放任务。 | 评测入口，不进入用户界面。 |
| `source_envelope_check` | 来源信封核验。 | 评测入口，不进入用户界面。 |
| `workbench_closure_check` | Workbench 闭环核验。 | 评测入口，不进入用户界面。 |
| `release_functional_closure` | 发布级功能闭环核准。 | 完整插件交付能力，不作为用户可见模式名。 |
| golden / oracle / rubric | eval-only 评测材料。 | 不公开、不作为事实来源。 |

公开文案规则：

- manifest `defaultPrompt` 只能提供稳定入口示例，不能写内部 intent、eval task、过程性标签或测试术语。
- README 对用户展示能力时，使用“开放式深挖”“专题线索”“资金穿透”“证据复核”“报告门禁”等产品术语。
- Agent 用户可见输出不得写内部 task id、oracle、rubric、diagnostic label、route violation 或 assertion marker。
- Investigation Lab 卡片标题应表达“开放式假设研判”“不规律模式线索”“证据缺口与下一步”，不表达历史过程来源。
- eval、doctor、debug trace 可以保留内部名称，但必须停留在本地质量证据或程序字段中。
- 公开文案不能承诺法律定性、最终资金归属、资产权属、实际控制或自动定罪。

公开文案落地核准：

| 文案面 | 核准要求 |
| --- | --- |
| Manifest interface | displayName、description、defaultPrompt 只能出现产品能力和稳定入口，不出现内部 intent、eval mode、assertion marker 或历史过程表达。 |
| README | 面向用户的命令说明使用产品术语；变更记录、工程细节和质量证据不应压过能力说明。 |
| SKILL.md | 保持短入口和硬边界；内部 intent 只能作为必要路由字段出现，不应成为用户可见标题。 |
| Answer / Lab Card | 标题、一级标题和正文使用“开放式深挖”“假设队列”“证据状态”“下一步追查”等产品术语。 |
| Report / Attachment | 使用经侦材料表达，不出现 debug、oracle、rubric、route violation、task id 或内部评测语言。 |
| Doctor / Eval Output | 可以保留内部字段，但必须脱敏，且不得被复制进用户报告或公开页面。 |

公开文案不只是语言问题，它会影响用户是否把插件理解成商业研判能力，还是误解成围绕某个历史过程或测试任务的临时工具。后续改 UI、README、SKILL、card renderer、Lab title 和报告模板时，必须按本节核准。

## 16. 商业成熟完成定义

Analytix 涉案资金研判插件达到顶级商业成熟状态时，应同时满足：

- 插件身份清晰：Skill + MCP + references + scripts + assets 形成完整能力包。
- 默认 source-backed 能力层可见，完整低层工具面受控。
- Capability Registry、Command Router、Command Metadata、tool schema、doctor、eval 一致。
- 自然语言侦查意图可由 Codex 直接选择语义事实工具；复杂任务再进入 Task Profile、Intent AST 和 Plan DAG。
- 金额、笔数、账户、户名、对手方、交易边、claim 都有 Evidence Ledger。
- Context Compiler 给模型最小充分研判事实，raw payload 不进正文。
- casegraph 成为案件事实底座，fundgraph 只承载 supported transaction edge。
- Investigation Lab 支撑开放假设、多轮追查、反证和收口边界。
- 资金链路穿透能覆盖来源、去向、一跳、下一跳、多跳、现金桥、混同资金、金融产品、资产端、支付通道和补调闭环。
- 报告、附件、Mermaid、续调清单和重大 claim 都能复核。
- Anti-pattern Detector 和 Critique Pass 覆盖 ask、rank、lab、flowgraph、report、evidence、review。
- 被动非资金任务保持 passive，不污染普通 Codex 工作。
- doctor、eval、health、trace replay、report review 能证明事实准确、追查深度、幻觉抑制、交付质量、成本治理和隔离边界。

最终形态不是“更会查事实的插件”，而是“证据约束下的开放式涉案资金研判能力层”：它让 Codex 保留侦查员式的问题意识、怀疑能力、追查方向选择和材料组织能力，同时用确定性事实、证据账本、图谱支撑、上下文编译和报告门禁，把自由探索升级为可复核、可交付、可治理的商业成熟插件能力。


### Conditional Release Blocker Notes

- Conditional release blocker: Pair Amount may ship only when focused owner routing, support-layer hiding, and final Chinese amount review all pass functional-eval.
- Conditional release blocker: Visual/report artifacts may ship only when generated surfaces are inspected and delivery gaps are explicit.
- Conditional release blocker: Current-case workflow continuation may ship only when case sync, cleaned export, report patching, and attachment handoff remain source-backed.
