# Agent 工作指南比较：2026-07-29 历史记录

Status: Historical / reference

从文档地图迁出，保留原比较、固定引用和当时数量；本次迁移没有重新验证
这些外部结论，不构成当前施工规则。当前入口是 [文档地图](../README.md)。

## 外部 Agent 原则借鉴

2026-07-29 通过当前 Codex 官方手册复核：

- [AGENTS.md discovery](https://learn.chatgpt.com/docs/agent-configuration/agents-md)
  从项目根目录向启动时的当前工作目录逐层加载文件，越接近工作目录的规则越晚
  生效；组合内容默认受 32 KiB 上限约束。根指南只保留每项任务都需要的仓库级
  目标、事实源和安全底线，目录差异放入 scoped `AGENTS.md`，长流程放入按需文档
  或 skill。
- [Skills](https://developers.openai.com/plugins/concepts/skills)
  先暴露触发元数据，匹配任务后才加载完整流程。可复用但并非每轮必需的方法应
  利用这种 progressive disclosure，而不是变成根指南中的永久步骤。

同日固定官方实现与本机 upstream 快照继续复核：

- [`openai/codex` at `fa1d4c40`](https://github.com/openai/codex/blob/fa1d4c40d0e63eef2e0ba8a9e004ccd0a80b77f5/AGENTS.md)
  的可迁移部分不是 Rust 风格或固定行数阈值，而是 review invariants：model
  context 不重写历史、避免无谓 cache churn、所有注入项必须有硬上限；breaking
  change 审查覆盖 API、原始事件、CLI/config 和既有 session resume；agent logic
  变化优先在公开集成缝验证。其 `10K`/`1K` token 与 `500`/`800` 行阈值属于
  Codex 当前实现，不能直接变成 Analytix 的永久边界。
- [`anthropics/claude-code` at `7ef6eec9`](https://github.com/anthropics/claude-code/tree/7ef6eec9d9ba84ea6f233f26c45f1df5c5991843)
  的固定树没有根 `AGENTS.md` 或 `CLAUDE.md`，任务流程主要放在 commands、skills
  和 plugins。其[官方 memory 指南](https://code.claude.com/docs/en/memory)建议每个
  `CLAUDE.md` 目标小于 200 行、删除冲突和过期规则，并将低频流程按路径或 skill
  加载；[hooks 指南](https://code.claude.com/docs/en/hooks-guide)把固定时点、可机械
  判断的动作交给确定性执行层。官方
  [security-guidance](https://github.com/anthropics/claude-code/blob/7ef6eec9d9ba84ea6f233f26c45f1df5c5991843/plugins/security-guidance/README.md)
  采用 pattern warning、LLM diff review、agentic commit review 三层结构，也明确
  LLM review 可能漏报或误报；Analytix 吸收“机械门禁 + 模型判断 + 公开缝证据”
  的分层，不把某个 plugin、hook、模型或多代理配置写死进根指南。

`/Users/sun/Projects/_upstreams` 中 7 个含有效指南的顶层快照共计 104 个
`AGENTS.md`/`CLAUDE.md`、511,031 bytes（约 499 KiB）。它们是对照样本，不是
Analytix 的事实源：

| 快照 | 值得吸收 | 不吸收或需规避 |
| --- | --- | --- |
| [`Kun@f65ac05c`](https://github.com/KunAgent/Kun/tree/f65ac05c0f62d060b7a0cf208ee2941b6ecf41f7)、[`gajae-code@ee15071c`](https://github.com/Yeachan-Heo/gajae-code/tree/ee15071cb52316708a1a2235958ee7bfdd7738ec) | 明确产品边界、所有权、常见诊断入口和最小 workflow routing。 | 端口、完整路径清单、固定 workflow/agent 数量和仓库特定 Git 权限会快速漂移，应留在 scoped guide、runbook 或可执行 gate。 |
| [`hermes-agent@d7b36070`](https://github.com/NousResearch/hermes-agent/tree/d7b36070ef807841699ad32c5b6af547fee3ff64) | Desktop 指南按 authority 决定状态归属，区分 durable/runtime/lineage identity，并要求重试有界且以可恢复状态结束；根指南强调窄核心、边缘扩展和行为不变量测试。 | 根文件 1,433 行、75,142 bytes，把产品百科、命令、插件、配置和测试教程全部常驻；`Never give up` 一类无限坚持口号也不能替代显式停止或 blocked 条件。 |
| [`lazycodex@2a03f45f`](https://github.com/code-yeongyu/lazycodex/tree/2a03f45f1bbc1d5d0cbf6c1293888e9df3ccbebf) / [`oh-my-openagent@65715d1c`](https://github.com/code-yeongyu/oh-my-openagent/tree/65715d1c2c35e27ccf2195ef688b0909dddb403c) | scoped “where to look”、context compaction 和 session recovery 的 ownership map 有助于局部接管。 | 81 个指南共 310,489 bytes（约 303 KiB）；其中 64 个带 generated 快照，13 个含他人机器的 `file:///Users/...` 链接，根文件自身 37,076 bytes。动态计数、生成日期、机器绝对路径和过密嵌套会产生截断、漂移与冲突。 |
| [`opencode@849c2598`](https://github.com/anomalyco/opencode/tree/849c2598abc7d2b40261e74b5826bc74ffc78308) | protocol 文件的统一结构、provider 差异命名和 public-seam review checklist 便于审查复杂协议。 | 根文件与 `packages/llm/AGENTS.md` 合计 33,432 bytes，已超过 Codex 默认组合上限；大量语法偏好和动态架构细节不应常驻根上下文。 |
| [`instructor@47fdb2ca`](https://github.com/567-labs/instructor/tree/47fdb2ca07119d389a3c0e8bc28b9930b814f294)、[`claw-code@4ea31c1b`](https://github.com/ultraworkers/claw-code/tree/4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d) | 准确的入口命令、仓库形状和 provider propagation map 能缩短首次定位。 | 303 行的 contributor 教程重复 README/runbook；自动检测出的泛化模板缺少领域不变量。两者都说明“短”与“长”本身不能保证高信号。 |

这轮审计后，Analytix 根指南为 194 行；根与当前最大 scoped guide 的组合是
15,073 bytes（约 14.7 KiB）。新增内容只覆盖跨会话接管、不可削弱验证、确定性
规则下沉和 model-context/session-continuity 兼容面；动态状态、实现百科和低频
方法继续按需读取。

第三方仓库按当前固定 commit 作为行为设计输入：

- [`multica-ai/andrej-karpathy-skills` at `2c606141`](https://github.com/multica-ai/andrej-karpathy-skills/tree/2c606141936f1eeef17fa3043a72095b4765b9c2)
  提炼 think-before-coding、simplicity、surgical changes 和 goal-driven
  verification。Analytix 吸收其目标导向，但把“有歧义就停”和机械行数阈值改为
  风险判断：先查可发现事实，只有 materially different 的未决选择才询问用户，
  低风险可逆假设由模型说明后继续。该仓库是 Karpathy-inspired 社区项目，不是
  Karpathy 本人或 Analytix 规范；固定版本树没有顶层 `LICENSE` 文件，虽然
  README 与 skill frontmatter 声明 MIT，因此本次仅吸收行为原则，不复制文本。
- [`mattpocock/skills` at `2ab95809`](https://github.com/mattpocock/skills/tree/2ab958093e83e0ec752e6c1c5932da465bf23e0c)
  继续提供小型、可组合 workflow。相对此前审查的 `ed37663c`，当前差异仅为安装
  README，skill 正文没有变化。重点新增审查
  [`writing-great-skills`](https://github.com/mattpocock/skills/blob/2ab958093e83e0ec752e6c1c5932da465bf23e0c/skills/productivity/writing-great-skills/SKILL.md)、
  [`to-tickets`](https://github.com/mattpocock/skills/blob/2ab958093e83e0ec752e6c1c5932da465bf23e0c/skills/engineering/to-tickets/SKILL.md)、
  [`wayfinder`](https://github.com/mattpocock/skills/blob/2ab958093e83e0ec752e6c1c5932da465bf23e0c/skills/engineering/wayfinder/SKILL.md)
  和
  [`codebase-design`](https://github.com/mattpocock/skills/blob/2ab958093e83e0ec752e6c1c5932da465bf23e0c/skills/engineering/codebase-design/SKILL.md)，
  并继续参考
  [`diagnosing-bugs`](https://github.com/mattpocock/skills/blob/2ab958093e83e0ec752e6c1c5932da465bf23e0c/skills/engineering/diagnosing-bugs/SKILL.md)
  的 tight、red-capable feedback loop。

本项目分层吸收：

- 根 `AGENTS.md` 使用少量 pretrained leading concepts：outcome、source of
  truth、smallest complete change、tight signal、public seam。它表达成功条件，
  把工具、测试策略、原型、计划和 delegation 的选择交给当前模型按证据和风险
  决定。
- scoped `AGENTS.md` 只记录目录独有且稳定的 ownership、合同和验证差异，不再
  复制根规则、动态插件清单、阶段优先级、完整架构快照或 runbook。
- 复杂诊断、TDD、research、prototype、review、wayfinding 和 ticket slicing
  保持为按需 workflow。OpenSpec 后续可借鉴 `blocked-by`、acceptance seam 和
  executable frontier，但这些是计划表达能力，不是每个任务的强制仪式。
- 每条常驻规则都应通过 relevance/no-op/duplication/sediment 检查；能用正向目标
  表达时，不用连续禁令替模型枚举所有错误路径。

明确不吸收：每个任务都 relentless grilling、每个 test seam 都再次请求确认、
固定要求后台 subagent 或仓库研究文档、无条件 commit、merge/rebase 永不 abort、
一张 ticket 一次会话、作者个人 vault 布局，以及把设计启发式当作绝对架构法则。
这些规则会降低可逆工作中的自主性，或与 Analytix 的权限、工作区和证据治理
冲突。

没有安装或复制这两个仓库的通用 skills。外部仓库不构成工程事实、产品需求或
施工授权；任何实质性复制仍须经过固定 commit、license、provenance 和 admission
审查。
