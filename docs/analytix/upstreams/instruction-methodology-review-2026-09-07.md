# Agent 指令与施工方法复核（2026-09-07）

Status: Reference / scoped implementation decision record.
Scope: Codex instruction and skill maintenance; not Analytix product readiness evidence.
Current product requirements remain in accepted OpenSpec specs. This record neither installs an upstream framework nor authorizes publication.

## 采用结论

保留现有 Analytix RC Skill，改为支持 Sol、Astra 原生判断的协调工具。直接施工使用简短状态与适当验证；委派、跨任务接管、恢复才加载完整控制协议。相关任务可组成完整交付包，保留所有必要调用方、迁移和公共接口传播，不能把减少步骤解释成降低要求。

Superpowers 本轮保持 **reference-only**。研究其 14 个 Skill、许可证、Codex manifest 和通用 hooks；没有安装、调用其默认流程，没有复制上游脚本、prompt 或产品代码。已有 RC/OpenSpec/skill-creator 足以承载选中的方法，因此没有新增重叠 Skill。将来独立安装仍需 `agent-handoff-review` 要求的版本、manifest、hook、trust、prompt-token、prefix-hash 和 tool-schema 准入证据。

## 固定来源

| 来源 | 本轮固定状态 | 读取范围与借鉴边界 |
| --- | --- | --- |
| [Superpowers](https://github.com/obra/superpowers/tree/b36e0829c6d0140e93cfef2ca599b1b07d4a7797) | `b36e0829c6d0140e93cfef2ca599b1b07d4a7797`；Codex manifest `6.3.0`；MIT，Copyright 2025 Jesse Vincent | 14 个 SKILL 入口、README、LICENSE、manifest、hook 声明；方法比较，不引入实现 |
| [OpenAI Codex](https://github.com/openai/codex/tree/121f91fd5d9dc66017866ce9bdc49f1e182721df) | `121f91fd5d9dc66017866ce9bdc49f1e182721df`；Apache-2.0 | 全局指令读取、skills 配置、测试审阅入口；本机行为仍由实际配置与工具证明 |
| [Claude Code](https://github.com/anthropics/claude-code/tree/ab9b2cf7bb9e4f98ff264c07a22e46d83c29c558) | `ab9b2cf7bb9e4f98ff264c07a22e46d83c29c558` | README、LICENSE.md、plugin-dev 的 skill-development；行为研究，不复制受限源码或 prompt |
| [Astra 官方指导](https://developers.openai.com/api/docs/guides/latest-model) | 2026-09-07 在线复核，页面明确 `gpt-6-astra` | instruction following、initiative、skills 优先级、按风险验证；不照搬示例中的 PR/权限默认值 |
| [Codex Skills 文档](https://learn.chatgpt.com/docs/build-skills) | 前轮审计在线读取 | 精确触发、渐进加载、重复名称和配置；路径以实际本机 catalog 为准 |
| [Eric Provencher 的 Astra 文章](https://x.com/pvncher/status/2095991462416490862) | 前轮在浏览器读取 | 补充设计观点；不代替官方模型说明或项目授权 |

本轮观察到 Superpowers 的 Codex manifest 是 `hooks: {}`，通用 `hooks/hooks.json` 则声明 Claude SessionStart。不能把通用 hook 直接说成该 Codex manifest 已启用的 hook；整套引入的实际注入面仍需安装准入测试。

## Superpowers 逐项取舍

下面的“借鉴”是 Analytix 自有文字中的行为原则，不是将对应 Skill 安装为自动入口。

| Skill | 可用内容与落点 | 未引入的要求及原因 |
| --- | --- | --- |
| `using-superpowers` | 精确技能路由，落在 AGENTS 与现有 Skill 描述 | 所有对话、1% 可能相关就强制调用；会扩大触发范围 |
| `brainstorming` | 未决重大选择先明确目标和权衡；已在 OpenSpec/AGENTS | 每个任务都先批准设计、只升级流程不降级、逐节确认；重复索取已有授权 |
| `writing-plans` | 可独立验收的任务边界、明确必要接口与证据 | 每步 2–5 分钟、计划内写完整实现、固定执行方式菜单；限制主模型判断且重复源码 |
| `executing-plans` | 检查计划、依赖顺序与完整跟进 | 遇测试失败立即问用户、严格照每一步执行、强制 worktree；妨碍自主诊断 |
| `systematic-debugging` | 可证伪假设、追踪实际生产者/消费者、最小区分实验；RC diagnosis | 三次失败自动架构讨论、每个边界打印数据、必须读完整参考；可能泄密或扩大调查 |
| `test-driven-development` | 可行时先证明回归信号能检测原故障 | 无例外 RED、先写代码就删除重写、每个函数都加测试；不适用于所有任务且破坏已有工作 |
| `verification-before-completion` | 结论绑定实际候选、命令、环境和结果；RC evidence record | 每次消息都重跑 FULL command、局部检查一律无效；导致重复验证与错误扩大结论范围 |
| `requesting-code-review` | 独立审查用最小上下文和明确验收目标 | 每任务必派 reviewer、主 Agent 不可自己审 diff；没有风险条件 |
| `receiving-code-review` | 用证据裁定建议、拒绝无关新增功能；RC finding classes | 一项不清楚就全停、每条建议单独测、禁止特定礼貌词；与任务正确完成无直接关系 |
| `dispatching-parallel-agents` | 仅独立问题并行、有界输入、检查冲突 | 无条件一域一 Agent、集成必跑全套；不符合成本收益和单写者边界 |
| `subagent-driven-development` | 同类小任务合并、复用同代码证据、集中修复反馈、紧凑恢复记录 | 强制子代理、主 Agent 禁止实现、最低能力模型、5 轮后 parking 即完成、固定最终一轮；削弱底座能力或要求 |
| `using-git-worktrees` | 已有隔离检测、尊重宿主与用户工作区、保留脏文件 | 每次计划都建隔离、自动装依赖/全套基线、失败先问；Analytix canonical source 规则优先 |
| `finishing-a-development-branch` | 集成前明确目标状态、保留不属于任务的工作区 | 每次全套测试与固定 merge/push 菜单；已有 local commit 授权不应重问，外部操作仍需明确授权 |
| `writing-skills` | 精确触发、渐进引用、现实场景验证；本轮用现有 skill-creator | 每次编辑必基线 RED、每变体至少 5 次、删除未经测试文字、默认 push；把改指令变成新仪式 |

## RC 与 OpenSpec 改造后的决策边界

- 直接施工无需虚构 Owner 路由、lease packet、event/barrier 或逐文件 Slice；真实委派与写入权转移保留完整协议。
- 主模型与 reasoning effort 不被 Skill 修改或限制；Luna 只在现有配置和授权条件下作为可选角色。
- 历史 milestone、一次性命令配额与 epoch 指示不变成项目永恒规则；明确永久退休的 task identity 仍不能恢复写入。
- 所有机器事件先匹配 epoch/brief/writer，再比较 revision、generation、sequence。自然语言 Owner 批准只在唯一且无歧义的待决项上记录映射。
- 证据绑定候选内容及相关生成输入、命令、环境、执行者和 claim；没有相关变化不重复跑同一套。正式 exact-artifact 规则保留。
- 失败实验若排除假设仍可有价值；无新证据的重复才触发重评。未解决的必要问题不能通过轮数耗尽转成完成。
- `agent-handoff-review` 用 OpenSpec 正式变更更新：有界 brief、可追溯结果保留，只有持久准入/交接或大型证据需要独立文件，不再因空模板字段或冗余封装阻塞。
- Skill 不能把用户的本地交付请求变成外部注册、上传、发布；已明确授权的同一目的地和受众不重复确认。

## 本轮验证的含义

执行 canonical OpenSpec Skill 检查、Skill 元数据/语法检查、文档链接与 diff 检查，并进行独立情景推演。检查结论以本轮交付报告为准。没有运行与文字改造无关的 Go 全量矩阵、桌面构建、正式打包或真实 Provider 调用。

这些证据支持指令结构和决策一致性；不代表已经测出项目交付速度提升，不构成 Product 或 Formal RC 验收。插件缓存升级可能替换本地文本，应按保存的精确版本和差异复核，不自动向新版本重放旧补丁。
