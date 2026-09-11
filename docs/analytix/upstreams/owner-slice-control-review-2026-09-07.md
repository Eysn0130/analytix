# Owner / Slice 协调复核（2026-09-07）

Status: Reference / dated instruction-maintenance and recovery audit.
Scope: 当前技能治理、宿主工具能力和两个中断任务的恢复入口；不是产品验收。
规范来源：[RC Skill](../../../.agents/skills/analytix-rc-control/SKILL.md)、
[agent-handoff-review](../../../openspec/specs/agent-handoff-review/spec.md)。
本记录补充[前轮方法取舍](instruction-methodology-review-2026-09-07.md)，不改写前轮历史。

## 再复核结论

前轮提交 `49c51d45aa1cf32c620d2a16ef6f4de14d391194` 的方向正确：按任务触发、保留底座模型能力、完成已授权目标、风险相称验证、区分真实权限和流程措辞。开工时核对其私有清单中的 94 个手动修改文件，均与前轮 after 哈希一致；这证明文件未漂移，不证明指令语义全部正确。

本轮进一步复核正文与必要直接引用，确认仍有残留：部分技能入口已收窄，正文后段仍重复批准、强制全量验收或清理重建材料。因此前轮“已全部闭合”的理解过强。本轮在原规则位置修正残留，并补充 Owner/Slice 运行方式；不靠一个总括例外掩盖互相矛盾的正文。

## 当前研究依据与取舍

| 来源 | 精确来源 / 本轮观察 | 采用与限制 |
| --- | --- | --- |
| [Astra instruction following](https://developers.openai.com/api/docs/guides/latest-model#instruction-following) | 2026-09-07 官方现行指导；技能冲突会导致提前停工，明确用户指令优先 | 明确作用域及已有授权；保留真正未授权操作边界 |
| [Astra testing](https://developers.openai.com/api/docs/guides/latest-model#testing-and-verification) | 应按改变及风险验证，通过后仅在新变化、失败或未解疑虑需要时扩大 | 复用同候选证据；不把少测等同于降低验收 |
| [Codex subagents](https://developers.openai.com/codex/subagents) | 主任务保留需求与决策，隔离执行噪声；配置以当前工具和实际角色为准 | Owner 保持精简、Controller/Slice 承担执行；不套用旧文档里的模型推荐 |
| [OpenAI Codex 源码](https://github.com/openai/codex/tree/21bd5d3cdcf6a6a22112a5a9571d00669af8ed05) | 本轮 pin `21bd5d3cdcf6a6a22112a5a9571d00669af8ed05`；Apache-2.0；multi_agents_v2 的 send_message 为 QueueOnly，followup_task 为 TriggerTurn；app-server 的 turn/completed 包含 completed/interrupted/failed | 区分送达、唤醒、结束、成功；按宿主当前可调用 schema 操作。未引入源码、hook 或新工具 |
| [Codex scheduled tasks](https://learn.chatgpt.com/docs/automations) | 同任务定时跟进和独立任务不同；本地运行需机器和应用可用 | 授权后只给活跃 Controller 配一个低频 heartbeat，使用原生工具；不写 TOML 或偷偷新建 cron |
| [Claude agent teams](https://code.claude.com/docs/en/agent-teams) | 自动通知、共享任务协调；更多代理增加协调和 token 成本，实验功能有恢复限制 | 借鉴关键事件通知和责任边界；不导入实验编排层或声称 Codex 有相同恢复能力 |
| [Claude scheduled tasks](https://code.claude.com/docs/en/scheduled-tasks) | 调度能力和会话生命周期有具体条件 | 参考定时回查思路，不把 Claude 的命令和持久性假设套到 Codex |
| [Claude Code 仓库](https://github.com/anthropics/claude-code/tree/ab9b2cf7bb9e4f98ff264c07a22e46d83c29c558) | pin `ab9b2cf7bb9e4f98ff264c07a22e46d83c29c558`；阅读 agent-development 与 LICENSE.md | 仅行为研究；不复制受限代码、技能 prompt 或资产 |

Exa 用于发现与抓取相关官方页面；结论依据实际读到的官方正文和固定版本源码，不按搜索请求的 numResults 声称审阅数量。前轮已审阅的 Superpowers 14 项取舍继续有效：保持 reference-only，不安装整套，也不添加重叠的全局技能。当前缺口是本地协调约束，不需要额外框架。

## 实际有差异的改造

- **Owner 的工作**：维护已接受目标、优先级、保留决定和最终交付；Controller 自主处理普通归因、修复、任务排序和复核。角色可以合并，不强制三层指挥链。任务下发不等于产品发布授权。
- **追加 Slice 任务**：同一交付包的澄清通过更高版本的 CONTINUE；需要稳定候选时 REVIEW_SEAL 后集中 REVISE。非阻塞美化不进入必修。正常验收结束可以使用双方事前支持的 COMPLETE_BRIEF；旧 brief/lease 永久关闭，健康任务可在新基线、新 brief、新 lease 下复用。
- **保留退役保障**：旧 TERMINAL_FREEZE、CANCEL 和永久退役身份不能被新技能复活。未声明新能力的旧参与者保留旧关闭协议。当前用户明确的一次性命令、禁止重跑和精确路径限制不能被总工“重置预算”。
- **心跳**：关键事件优先；长任务按需从约 30 分钟开始选择回查频率，30–60 分钟是示例而非死线。只设置一个活跃 Controller 监控；无变化静默、结束本次检查，不逐 Slice 建观察者、不反复读历史、测试或 sleep。安静运行仍消耗资源。
- **读状态**：保存每个目标的 cursor；一次 wait 可能只返回第一个结束的目标。已处理终态从后续 wait 集合移除，防止旧失败反复抢占。读取前先筛选结果，省略 preview、推理和无关工具历史。
- **上下文交接**：依据状态重建难度、偏航和验收可信度决定，不能按固定 token/轮数强制换工。旧命令停止、lease 撤销、监控暂停后，新总工进行 BOOTSTRAP 再接管。更换 canonical Owner 本身仍需明确授权。
- **交付收敛**：每个必需行为有证据或明确缺口；focused green 不能关掉未完成的产品、迁移或包义务。检查只有能改变下一步或支持所需结论时才继续扩大。

这次新增的有限行为能力是“正常结束 brief 后复用未退役任务”和“获授权后协调低频心跳”。它们不增加产品源码范围、外部写入、凭据、破坏性操作、发布权限，也不修改主模型、effort、sandbox 或 approval。它们不能撤销旧任务的明确限制。

## 两个中断任务的身份与状态

| 用户所指 | 本轮工具确认 | 处理结论 |
| --- | --- | --- |
| Owner `01a04667-5661-7f40-b2a3-0e883fbcaa49` | `Analytix Owner Command｜最高项目指挥`；idle，最近一轮 failed / usage limit | 仍是 canonical Owner；历史失败不证明当前额度仍不足 |
| 用户贴作 AH 的 `01a079a5-fcc3-7303-a121-9ac508b28270` | 实际为 `审计 AGENTS 与 Skills 指令` | 这是当前审计任务，不能拿该 id 作为 AH 恢复入口 |
| 从 Owner 记录找到的 AH `01a05784-0d67-77b1-95ff-adb8929a6eb9` | `Analytix Controller Epoch AH`；idle，R128 最近一轮 failed / usage limit | R128 未形成最终归因交付，须先对当前状态重新绑定 |

Owner 返回的最近一轮 500-item 片段中，包含 132 次 wait_threads、3 次 read_thread、9 次 send_message_to_thread。这个有上限的返回片段证明重复等待模式存在；它不是完整历史、账单或精确 token 消耗。长 preview 和大量 items 也说明 turnLimit 很小仍可能带回很大上下文。

THREAD-REPORTED：R127 普通 Go 矩阵失败；首个记录为 runtime-server 构建子进程 signal: killed，未确定是 OOM、测试超时或产品缺陷。旧 R128 授权仅作保留证据和精确源的只读归因，禁止重跑、额外验证和产品修改。

OBSERVED：R128 私有目录中的 classification.closed、source-chain.closed、log-order.closed、terminal.closed、evidence-manifest.txt、fingerprints.closed 均为 0 字节。文件存在不能证明归因结束。当前 canonical HEAD 已包含本次之前的治理提交，不能机械重放绑定旧 HEAD 的 bootstrap。未执行完整产品 WriterLease/FD BOOTSTRAP，因此本审计不宣称可以立即授予源码写权。

OBSERVED：一次对两个已知任务的原生 wait 快照只返回 Owner，wake=turnCompleted 且实际 turn 状态为 failed。这验证了监控必须检查状态、游标及返回目标范围。8 个已存在的自动化均 PAUSED，无 Owner/AH 当前绑定；保留原状。本轮准备心跳模板，不启动后台监控或产品线程。

## 恢复顺序与验证边界

从正确 Owner 任务恢复；读取新 Skill 和本记录，保留旧 R128 的只读/no-rerun 边界，将治理提交后的 canonical 状态与原 R126/R127 保留证据重新绑定。关闭仍缺的 R128 首失败归因，不把空文件或旧 commentary 当结果。只读归因若仍无法确定原因，如实保留不足及精确下一项决定，不凭推断重跑整套矩阵。

随后在既有授权内选择下一依赖有效交付包；只有安全交接证据齐全，才移交新 Controller 或授予 Slice 写权。若现场继续需要无人值守监控，再按当前有效任务身份与授权创建一个 heartbeat。无需把本审计任务改造成总工。

验证限于指令语法/链接、当前 OpenSpec 生命周期、受保护工作区哈希、独立场景推演和上述一次原生只读状态检查。没有执行 Go 全量矩阵、桌面运行、正式打包、真实 Provider、活跃心跳或产品恢复；不据此承诺量化提速或产品/正式 RC 已通过。详细宿主修订和验证状态保存在本机私有 follow-up 交付记录。
