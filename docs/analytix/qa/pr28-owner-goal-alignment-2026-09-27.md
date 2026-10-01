# PR28 原目标、施工交付与 Owner 纠偏

Status: Reference / 2026-09-27 的目标对照与决策记录。
审查基线：`71ece7c971a423677985f63b65aef720e2f120c8`；产品 SOURCE
`11e6fff85a72934e30ed805a4fcf5a8638fc7623`。后续产品施工状态仍由
[接续 ledger](pr28-next-execution-2026-09-26.md) 维护，本文件不另建执行台账。
用户本轮委托最高 Owner 自主核对原始要求、纠偏施工并改进 RC Skill。
这不扩张真实用户数据、凭据、外部写入、合并或发布权限。

## 原始材料与目标

读取引用聊天《解释连续成功回合》的相关内容，核对了两份上传附件，
并找到三版施工提示词，逐项审查完整 post-c8 及 N01–N09 后继要求。材料身份如下；SHA 是文件摘要，
不是产品提交。两份上传附件均与下列仓库记录逐字节一致。

| 材料 | SHA-256 | 处置 |
| --- | --- | --- |
| `Analytix-DSH-e15-Next-Codex-Prompt-20260926.md` | `4d807867af98ebf24ae2af4f0f54075cbd7e3989ab669b26a748d82c700648fb` | 七项工作、四步推进的前序研究；任务组织被下一份替代，既有安全约束保留 |
| `Analytix-PR28-Post-c8-Completion-Codex-Prompt-20260926.md` | `3762df07a1e767bb6245b7c04a68136ef6f61cdb805e97957b8ddd8159440062` | D01–D08 的固定工程交付要求；历史 SHA 不能成为回退目标 |
| `Analytix-PR28-Next-Execution-Prompt-20260926.md` | `edeeafaa7d122112761ebc928f05287a7f250b28c0233ec03205a372349c52fd` | 74e 后继 N01–N09：Core 安装/性能与 Funds 正向闭环；保留前轮有效实现，不取消 D04 正确性 |
| [e15 performance 附件](pr28-e15-performance-followup-2026-09-26.md) | `ebe5c12fe42ab3a0f29fedcf05659beb7407126b3732582d0a6713371bd1174c` | c8 的局部修复、微基准与当时未完成项 |
| [e15 credential 附件](pr28-e15-credential-authority-2026-09-26.md) | `6add68b3b2ae0792cb0b17c203c1da2de8a48883c5eb7328b6c44c52610154f5` | e15 合成安装、升级及恢复范围；不是后继验收 |

原目标是同一模型、协议、推理档位和任务下，更早得到有效结果，以更少的
无效输入与往返完成工作，并保持权限、敏感数据边界、任务正确性及恢复。
不是追求提交数、固定 OK 回合数、最高缓存百分比或最小局部函数耗时。
原提示词明确：完整候选发布合同继续保留；未确认的 DSH 扩展允许受支持
回退；真实 Provider 缺口只阻塞对应检查；全面超过 DSH 不增加为 PR28
合并前置条件。

## D01–D08 是否实际执行

以下源码观察对应审查基线；测试和安装结果引用其明确历史 SOURCE。
本轮只读审查没有重跑这些产品检查，不将历史绿灯转给最新源码。

| 项目 | 已有实现及实际消费链 | 当前结论与缺口 |
| --- | --- | --- |
| D01 身份与交付记录 | [post-c8 记录](pr28-post-c8-completion-2026-09-26.md) 与接续 ledger 保留 source/artifact 区别；既有提交在当前分支祖先中 | 已执行；本地后继与远端 CI 继续分别报告，旧摘要不能充当当前完成结论 |
| D02 编码、预算、连接 | [Chat request](../../../packages/runtime-go/internal/adapters/outbound/provider/openai/request.go)、[Messages request](../../../packages/runtime-go/internal/adapters/outbound/provider/anthropic/request.go)、[client](../../../packages/runtime-go/internal/adapters/outbound/provider/client/client.go)；生产 runtime 注入并在 drain 后关闭 | low/cap/连接池改动仍在；HTTPS、认证头轮换、race 与生产装配有原候选证据，不应重新实现 |
| D03 发布到画面 | [publication trace](../../../packages/runtime-go/internal/server/publication_trace.go)、Main trace、[真实消息组件](../../../src/renderer/src/components/chat/message-timeline-bubbles.tsx) 的 DOM 提交；74e 有安装 T01–T08 | 有生产接入及历史安装结果；DOM、rAF、合成截图、物理显示时刻仍是不同证据。新源码安装验收不能借用 74e |
| D04 缓存与费用真实性 | [最终 wire 分段](../../../packages/runtime-go/internal/domain/cachetelemetry/input_segments.go)、[attempt telemetry](../../../packages/runtime-go/internal/adapters/outbound/provider/client/cache_telemetry.go)、[usage](../../../packages/runtime-go/internal/app/usage/cache_observations.go)；`not_checked` 与估算器语义已修正 | 主要实现已执行，但本轮发现费用聚合和 UI 消费缺口，当前完整关闭结论须重开，见下节；真实 KV 收益未证明 |
| D05 Messages | [compat](../../../packages/runtime-go/internal/adapters/outbound/provider/compat/request.go) 到原生 enabled/disabled/effort、角色/工具配对、回放和流解析；Registry、设置页及既有测试消费 | 公共基线已实现；in-history/addition-only 并未形成 DSH 原生扩展全兼容。原提示词允许可信回退，不因此推倒重写 |
| D06 长会话语义 | [自动压缩](../../../packages/runtime-go/internal/app/turn/compaction.go) 保存版本化原始用户源；[Provider history](../../../packages/runtime-go/internal/app/model/provider_history.go) 与实际 `read_task_history` 消费；[回归](../../../packages/runtime-go/post_c8_protocol_continuation_test.go) 检查工具效果 | 已超出固定 OK 耐久测试，具有早期约束、两次压缩、更正及恢复证据；仍是有界合成矩阵，不是真实模型任意长会话语义无损 |
| D07 完整路径与热点 | [provider stream](../../../packages/runtime-go/internal/app/loop/provider_stream.go) 的增量预算/推理累积，Send 派生索引等待修复，以及固定阶段 UI | 74e 有 25 个最终受控样本、17 个匹配成功基线；冷热/历史差异限制因果结论，Main 重启样本变慢。不是同条件 DSH 胜出证明 |
| D08 准确安装候选 | 74e 的 DMG/ZIP、签名、许可、连续 120 与新 Main 第 121 回合、升级保全均有历史记录；后继 0ff 有独立 Full 安装诊断 | 实际生成过新候选，不能说一直停在源码。当前后继、受影响安装验证、真实 Provider、CI/PR 等仍各有缺口 |

`StableUserIDConsent` 的构造/校验及未授权不发送路径存在，生产激活 owner
在本轮搜索中未观察到。原 D04 明确允许默认不外发，故不因这个事实强制
新增设置、身份外发或休眠扩展。D06 原始源存在 256 条、1 MiB 总量及
16 KiB inline 阈值；超限拒绝而非静默丢早期限制。手动 `/compact` 不应被
自动压缩 V4 证据覆盖，参见[源合同](../../../packages/runtime-go/internal/domain/thread/continuation_sources.go)。

生产 [runtime runner](../../../packages/runtime-go/internal/app/loop/runtime_runner.go)
仍保留完整候选发布边界；Provider 首正文到达不等于用户首正文可见。
固定可信阶段、工具与恢复状态已经接入 UI，`response_received` 也不能被
称为首 token。安全分段正文发布如要改变，须另做协议、隐私与恢复设计；
本轮不以移除 final gate 换取表面速度。

## 新发现：D04 费用信息没有完整到达消费者

审查基线的两条确定性链路：

1. `EstimatedCostV1` 已有 `known/currency/nanoUnits`，但
   [cache aggregate](../../../packages/runtime-go/internal/app/cachetelemetry/service.go)
   只累加各币种金额与总 known 次数，丢失“金额为零但币种已知”的存在性。
   [thread hook](../../../src/renderer/src/hooks/use-thread-usage.ts) 又将零丢弃，
   `formatCost` 在 `priceConfigured=true` 时按 UI locale 选择币种符号。
   因而零费用币种会随语言变化；既有测试甚至断言了这一错误行为。
2. [usage records](../../../packages/runtime-go/internal/app/usage/records.go)
   用 OR 聚合 `PriceConfigured`；一个完整价格 turn 加一个未知价格 turn
   可成为“已配置价格”的普通合计。[usage index](../../../packages/runtime-go/internal/app/usage/index.go)
   未消费 attempt 覆盖信息。终态 owner 拒绝把不完整 attempts 作为完整
   费用本来是正确的，但可见聚合不能把部分小计重新包装成完整总额。

Owner 已将其交给现任唯一产品 writer，按既有成本 owner → terminal/public
contract → usage index/snapshot → thread/day/model hooks → heatmap 聚合修复。
不能只改符号或 UI 文案，也不引入第二套计费系统。有限验收集合：

- USD 零在中文、CNY 零在英文保持原币种；双币分别显示，不作隐式汇率换算。
- 全未知为未知；部分已知 attempt/turn 明确显示已知小计和未完成覆盖。
- 无 Provider 的空 bucket 不制造未知物理调用；取消后未知费用不被当成零。
- 旧记录缺少新可选字段时诚实降级；旧签名原文不被改写。
- index/restart/replay 不重复记账；严格公开 schema 拒绝非法覆盖与币种组合。
- 检查实际 hooks 和二次聚合，不止测试底层 helper。

本项派发不等于修复完成；产品提交和最终验证由接续 ledger 登记。

## 施工线程与上下文判断

本轮识别到的 PR28 相关主线程如下。运行配置按实际线程元数据核对，
不用线程名称或模型自述判断；这是读取时状态，后续以实时状态为准。

| 线程 | 核对结果 | Owner 处置 |
| --- | --- | --- |
| 接续执行 Analytix PR28 | Sol Max；正在协调 d244 诊断、D04 和后续 Funds 缺口 | 保留协调职责，明确原目标和产品验收边界 |
| PR28 唯一施工续接：运行时就绪与安装闭环 | Sol Max；N04 后继续 D04，N06 设计已形成有界提案 | 保留唯一产品 writer；费用合同已裁定，进入实际修复 |
| PR28 主线施工：重启后验证与合并闭环 | Sol Max；已结束该轮实施并由现任 writer 接续 | 保留历史证据，不另行唤醒写入 |
| 继续 PR28 恢复与验收；继续 PR28 修复与验收 | 历史 Sol Max 施工线，已有有效提交被当前祖先包含 | 不重放已集成补丁 |
| Analytix 通用 Agent Harness 基座交付 | 实际最后运行配置为 Astra xhigh，D01–D08 的历史交付来源 | 不能把所有已有工作都称作 Sol Max 施工；保留原候选证据 |

压缩次数、工时或聊天长度不能单独证明施工能力下降。本轮观察到的决策
等待和目标偏移可通过具体合同与顺序纠正；尚无足够证据认定现任 writer
无法可靠恢复状态或实现该包，因此不创建新主线程或复制完整历史。
若后续在明确 brief 后仍反复遗漏同一合同、重建错误候选或失去写入所有权，
才在可恢复边界交接简短证据包并轮换。

## Owner 决策与执行顺序

1. **保留现任 Sol Max。** 没有足够证据认定模型无法施工。它已修复当前
   N04 反例、撤回错误候选并给出有用的 authority 依赖映射。按结果和状态
   恢复能力判断轮换，不按压缩次数或累计工时决定。
2. **现在修 D04 消费链遗漏。** 它直接服务原目标，并可在合成环境完成。
   产品 writer 与 Owner 的 skill/report 文件范围分离，Git 集成串行。
3. **完成已有 d244 诊断的必要观察。** 原构建在原生编译后才暴露 archive
   缺 Git HEAD，属于应前置的构建身份检查。修复准确 checkout 元数据并
   复用有效产物，不降低快照守卫。该包只能回答其源码的 N03 问题。
   测量时排除编译争用，报告 observer 成本，不再纯延长超时重跑同包。
4. **跨 Main 物化互斥做最小隔离复现。** 当前内存 Set 和进程内 mutex
   尚未排除异常 Main 退出后的同根 writer 重叠；这不是已观察到损坏。
   先在临时根/合成 fixture 证明已有租约覆盖或最小反例，再修正确 owner。
5. **N06 保留现有 DSV2，不再等待错误二选一。**
   [accepted kernel](../../../openspec/specs/case-evidence-kernel/spec.md) 与
   [当前 tasks](../../../openspec/changes/case-evidence-publication-gate/tasks.md)
   保留 witnessed DSV2；延期的是新增独立 ThreadRisk/Registry witness。
   已接受方向是补普通安装可信 enrollment/bootstrap 与 SharedEvidence
   持久 owner/recovery，并审查解除 deferred ThreadRisk 的格式强制耦合。
   不删除 witness、伪造 enrollment 或改写历史签名。具体 signed schema、
   存量恢复/中断、持久头/密钥/floor 一致性设计由 Owner 审查，再进入施工。
   用户已委托工程决策，不再以抽象“按规范还是上 witness”等待用户。
6. **稳定候选后集中完成受影响验收。** 真实 Provider authority、当前 CI、
   合并和公开发布仍独立；普通研发可用性不能冒充安装 QA 凭据权限。
   Core 和 Funds 分别报告，Funds 缺口不取消 Core 已有证据或停掉独立修复。

既有真实用户实例被误终止的事件影响仍 unknown，见
[事故与后续限制](pr28-next-execution-2026-09-26.md#protected-user-state-incident-and-ui-stop-boundary)。
后续仅使用任务所属的准确 app/PID/birth/profile；本轮不访问真实用户状态。

## 上游复核与取舍

本次只读研究，不安装外部技能、不复制或移植其源码/提示词。两仓库根
LICENSE 在下列 pin 均为 MIT；这不构成未来任何文件自动获准复制的结论。
未来实际吸收仍按[上游准入规则](../upstreams/README.md)执行。

| 来源与 pin | 实际研究内容 | 本轮决定 |
| --- | --- | --- |
| [Superpowers `8ca22dba`](https://github.com/obra/superpowers/tree/8ca22dba9a94f28898bbce59f2537ff4d87c747d) | `systematic-debugging` 的可证伪诊断、`executing-plans` 的计划/代码冲突区分、`verification-before-completion` 的证据要求 | Reference Only：以本地已观察失败独立修改现有 RC Skill；不整包引入。固定“三次失败就架构错误”、每条消息重新跑完整命令、固定审批/角色流水线不适合作为本项目通则 |
| [DSH `21638c56`](https://github.com/deepseek-ai/deepseek-harness/tree/21638c56315ae6a2b552d6091945d3144c9af32e) | [`llm-deepseek`](https://github.com/deepseek-ai/deepseek-harness/blob/21638c56315ae6a2b552d6091945d3144c9af32e/packages/llm/llm-deepseek/README.md)、[`agent-loop`](https://github.com/deepseek-ai/deepseek-harness/blob/21638c56315ae6a2b552d6091945d3144c9af32e/packages/core/agent-loop/README.md)、[`session-log-deepseek`](https://github.com/deepseek-ai/deepseek-harness/blob/21638c56315ae6a2b552d6091945d3144c9af32e/packages/session/session-log-deepseek/README.md) | Reference Only：保留明确能力与稳定前缀的比较价值；不照搬默认 session log/plugin inventory 外发。比较范围必须覆盖整个 HTTP 外发而不仅 messages |

DSH 当前 pin 已在原 `477b4f4` 研究点之后；本轮没有执行其模型或端到端
基准，不将旧对照改称最新实测。它的能力声明、客户端实现和某个真实服务
端点行为不同。Analytix 当前的安全发布粒度也不同，因此比较须固定模型、
协议、effort、输入/输出、工具、网络、缓存冷热、权限和成功条件，并分别
记录首正文、合法可见结果、总任务时间、已知费用覆盖与错误率。
“有相似机制”不等于“速度/缓存已齐平”。

额外需要考虑的是冷启动与恢复、完整任务往返次数、未知费用覆盖、跨语言
显示一致性和观察器自身开销；这些可解释用户等待。不能仅优化 cache-hit
百分比或 Provider TTFT，也不把所有远期对照加入本轮产品门禁。

## RC Skill 修改与验证边界

本轮修改现有 [SKILL.md](../../../.agents/skills/analytix-rc-control/SKILL.md)
及 [Owner orchestration](../../../.agents/skills/analytix-rc-control/references/owner-orchestration.md)：
恢复原始附件/目标映射、追踪真实消费者、在重构建前检查廉价前置条件、
约束无信息增量的修补、先核实最小兼容方案再升级决策，并避免默认轮换。
不新增协议包、控制服务、强制角色或自动化。

独立只读前向推演覆盖四个情境：producer 已通过但 consumer 缺席；延期新
witness 与现有 witness 混淆；无 Git archive/共享构建资源/无真实 Provider；
健康线程多次压缩且仅文档变化。四个情境均能选择有界下一动作、保留权限
与证据边界；根据评审进一步澄清“有理由时才轮换”。这验证技能的决策
一致性，不等于已经观测它长期运行后减少了多少施工时间。

实际验证：同 shell 的 cache preflight 完成后，系统 skill-creator 的
`quick_validate.py .agents/skills/analytix-rc-control` 返回 `Skill is valid!`；
三份任务文件中的 43 个本地链接目标均存在，`git diff --check` 通过。
另行审查了两份 skill diff 与本报告，未修改 OpenSpec skills 或其验证器，
未重跑不受本次治理修改影响的产品测试。本轮不生成新安装包。
产品 D04 修复、N03 制品与 N06 的完成结论不得由本报告或 Skill 通过代替。
