# analytix 文档地图与证据治理

本页解决一个长期风险：项目中的 Markdown 同时包含当前规范、施工方案、
迁移历史、上游调研、时点验收、生成报告和打包副本。文件被 Git 跟踪、搜索能
命中或修改时间较新，都不代表它是当前事实来源。

## 当前交接与整理入口

- [`development-baseline.md`](development-baseline.md) 是当前公开主线开发入口：
  `pull` 后的依赖更新、工具链、资源准备、CI 与封包的已实现边界。
- [`git-workflow.md`](git-workflow.md) 是 canonical 与 GitHub 公开主线的
  `pull` / `commit` / `push` 操作入口，也说明保留的私有历史与资源排除范围。
- [`handovers/README.md`](handovers/README.md) 是跨线程暂停和恢复的操作入口。
- 当前施工快照与补充记录统一由
  [`handovers/README.md`](handovers/README.md) 路由；本页不再重复维护“最新”文件名。
  PR #28 未合并时，先 fresh 读取 PR 的 head 分支，再在该分支读取交接入口，
  不假设默认 `main` 已包含候选分支的记录。
  `2026-09-10-owner-replacement.md` 保留为 R131/B1 与旧 Owner 交接的历史快照。
  恢复时仍须从当前 Git、active OpenSpec、代码和 fresh 验证重建状态。
- `2026-08-05-damaged-cache-retirement.md` 仍是损坏缓存退役的历史来源，
  不是当前 HEAD、容量、writer 或施工顺序的依据。
- `2026-08-04-controlled-thread-checkpoint.md` 中的旧镜像保留和 backing 容量
  blocker 已被 `2026-08-05-damaged-cache-retirement.md` 取代，其余内容只作
  Historical 路由背景。
- `2026-07-25-first-stage-pause.md` 和
  `2026-07-25-thread-close-route-audit.md` 均为 Historical，只保留当时时点的
  工作区与路线背景，不再作为当前恢复入口。
- [`document-consolidation-register.md`](document-consolidation-register.md)
  记录大文档、历史材料、碎片和重复副本的分类与后续归并队列。
- [`development-runbook.md`](development-runbook.md) 是缓存、验证、打包和启动
  诊断的当前命令入口；命令被列出不等于已经通过。
- [`knowledge-base.md`](knowledge-base.md) 定义 GitHub 工程事实源、Notion 有界知识
  镜像与 Google Drive 正式交付层；Obsidian 仅保留为 legacy，不自动同步或迁移。
- [`../legacy/README.md`](../legacy/README.md) 明确 `docs/legacy/` 只作历史
  provenance，不能作为当前 Analytix 实现入口。

## 先区分三个问题

| 问题 | 事实来源 | 发生冲突时怎么处理 |
| --- | --- | --- |
| 当前工作区实际做什么 | 当前代码、测试、`package.json` scripts、构建与打包配置、可复现运行结果 | 文档与代码不一致时，记录为文档漂移或未实现目标，不把文档描述冒充现状。 |
| 产品应该做什么或必须保留什么 | [`specs/README.md`](specs/README.md) 登记为 accepted target/acceptance contract 的 specs，以及 `openspec/specs/<capability>/spec.md` 中已接受的 scoped requirements；active OpenSpec change 是 scoped proposal/work plan | 较新的、范围更具体的已接受规范覆盖同一主题的旧目标；active change 只有被当前用户请求批准、apply 或 continue 后才成为施工指令。 |
| Agent 在仓库里应该怎样工作 | 从根目录到目标目录路径上所有适用的 `AGENTS.md`，冲突时以更具体的规则为准 | 工作规范不能用来证明产品行为已经实现。 |

如果“当前实现”和“目标规范”不同，正确结论是存在 drift。先说明：

```text
as-built: 当前代码和验证观察到什么
target: 当前规范要求什么
gap: 哪个传播层、测试或证据仍缺失
```

不要通过修改文档把 drift 隐藏掉，也不要在只获授权分析文档时顺手修改产品
行为。

## “总控”与 Controller Epoch 推进方式

“总控”表示任一时刻只有一个最高调度与验收 authority，不表示一个
无限延长的会话、一个另外的 runtime，也不表示用历史交接替代 fresh
evidence。当前施工遵循以下最小路由：

1. 从适用的 `AGENTS.md`、当前 Git/worktree、accepted target、active
   OpenSpec 和 fresh gate 重建事实，不继承历史 PASS。
2. 按可交付结果组织有有限终点的施工；直接施工使用简短记录，委派或跨任务
   控制才使用 Slice 协议。相关任务可合并验证，不为每个文件建立新流程。
3. 旧全仓独占 lease 保持单一 writer；新 brief 只有按 RC Skill 双方明确采用
   scoped 并行、证明文件及运行资源不冲突后才允许独立写者。只读审计也须读取
   稳定候选；最终共享 Git 集成和验收串行，由当前总控负责。
4. 严格区分 focused candidate、product acceptance 和 formal release
   acceptance；证据不足时保持 `partial` / `blocked` / `unverified`，不用
   计划、文档或一次测试代替。
5. 只在用户明确要求时使用 Codex Goal 跟踪；Goal 不是 Analytix 产品架构、
   OpenSpec 状态或验收 authority。
6. 一个 Controller Epoch 是有限调度周期，不是永久 owner。当上下文、范围、
   HEAD/writer 状态或 next action 开始不健康时，在原子命令安全边界停止
   新增写入，交付当前事实、未完成分母和下一个 Slice，再由新 Epoch fresh 恢复。

## 外部 Agent 原则借鉴

原 2026-07-29 比较记录及其固定引用、数量快照已迁至
[历史参考](upstreams/agent-guidance-review-2026-07-29.md)。它用于解释指南设计，
无需在普通任务中反复加载。当前工作规则来自适用的 AGENTS.md 与任务匹配的
Skill；外部实践不能增加产品需求或权限。

## 当前架构锚点

以下锚点用于快速排除最危险的旧判断；具体行为仍需查看当前代码：

- 用户可见品牌是 **Analytix**，产品类别是 **Agent Platform**，核心品牌句是
  **Analytix — Agents for sensitive work.**；机器级 package、executable、CLI、
  protocol 和 env 身份继续使用 lowercase `analytix`。
- 对外架构统一表达为 **Go Agent Harness + Plugins + Privacy Layer**。三者属于
  同一个 Go runtime：Privacy Layer 是不可绕过的内置边界，Plugin 扩展专业能力，
  都不创建第二 Agent runtime 或第二 authority family。完整目标与 current gap 见
  [`specs/11-agent-platform-brand-and-architecture.md`](specs/11-agent-platform-brand-and-architecture.md)。
- 桌面主权边界是 Electron main / preload / React renderer / TypeScript shared
  contracts。
- 普通启动使用本地 Provider Registry / Secret Store 和正常引导；Hub 只在显式
  兼容操作中延迟加载。旧 Hub-first 验收方法不能覆盖已接受的本地凭据规范。
- renderer bridge 是 `window.analytix`；runtime-owned settings 使用顶层
  `runtime`，模型 provider 配置使用顶层 `provider`。
- 生产 agent core 是 `packages/runtime-go`；组合根是
  `packages/runtime-go/internal/runtimeapp`。
- Funds 是首个旗舰专业插件，不是主品牌、产品类别或 Core。Knowledge、Legal、
  Research、Writing、Coding 等专业能力沿同一个 plugin capability/lifecycle
  契约扩展。
- `packages/runtime` 提供公共 contracts/config/telemetry 与
  `analytix serve` TypeScript launcher；launcher 启动 Go
  `runtime-server`，不再承载生产 TypeScript agent loop。
- `ANALYTIX_RUNTIME_BACKEND=typescript` 是 retired-backend 诊断路径，不能
  启动 TypeScript agent runtime。
- 公共产品协议仍是 analytix HTTP/SSE contract；renderer 不感知 Go 私有路由
  或上游产品协议。
- canonical package/executable/app id/CLI/env identity 是 `analytix`；official
  Standard Windows 只在 display copy 使用 `Analytix灵鉴`，artifact 使用
  `analytix-standard-${version}-${arch}.${ext}`。
- 新交互会话的 policy 默认组合是 execution-policy version 2：
  `approvalPolicy: on-request` 加 `sandboxMode: workspace-write`。未版本化且恰好
  是旧默认 `auto` 加 `danger-full-access` 的组合只迁移一次；其他有效选择和
  version-2 full-access opt-in 保留。workspace-write 是应用层 tool/path
  policy，不是 OS 沙箱，并会阻止前台/后台主机 shell。无人值守 Connect Phone
  与 scheduled task 使用 `never` 加 `workspace-write`。

任何文档只要把缺失的 `packages/runtime/src/server/runtime-factory.ts`、
`packages/runtime/src/loop/agent-loop.ts`、
`packages/runtime/src/adapters/model/deepseek-compat-model-client.ts`，或
`packages/runtime/src/server/routes/` 当作生产修改入口，就已经是旧架构说明。

## 规格地图

[`specs/README.md`](specs/README.md) 是编号 specs 的 canonical registry，负责
标记 accepted target、accepted acceptance contract 与 historical reference。
它取代在多个入口手工维护编号数量和状态的做法；新增、替换、接受或退役
spec 时必须在同一变更中更新登记表。

同一主题出现冲突时，先查登记状态、该 spec 的 currentness/addendum 和范围更
具体的 accepted requirement，再查当前代码和 focused tests。不要按文件编号、
行号更靠后、Git 跟踪状态或修改时间机械决定权威性；登记为 accepted 也不证明
当前实现已完成或 fresh validation 已通过。

## OpenSpec scoped requirements

`openspec/specs/` 与 `openspec/changes/` 是另一层 scoped governance，不替代
上面的产品总规格：

- `openspec/specs/<capability>/spec.md` 是已同步接受的 capability requirement；
- `openspec/changes/<change>/` 是 active proposal/work plan；只有当前用户请求
  明确批准、apply 或 continue 时才成为施工指令。artifact status `done` 只说明
  proposal/design/spec/tasks 已生成，不说明 implementation tasks 已完成；
- `openspec/changes/archive/<date>-<change>/` 保存已归档 change 的决策和施工
  记录，不是永久通过证明；
- delta spec 只有在 archive/sync 后进入 `openspec/specs/`，未同步的 active
  delta 不能冒充 main requirement。

不要在本页手抄动态 change 清单。三份重点方案的 2026-09-09 审阅见
[文档与交付审阅](documentation-delivery-review-2026-09-09.md)，它不是实时进度表。
需要当前状态时运行；在配置的 macOS 主机先于同一 shell 加载缓存脚本：

```bash
source ./scripts/use-analytix-cache.sh
openspec list --json
openspec list --specs --json
openspec status --change <change-name> --json
openspec validate --specs --strict
```

引用某个 change 的实现结论前，还必须读取其 `tasks.md`、检查当前 diff/代码，
并确认适用验证的候选、环境和覆盖仍有效；只重跑已失效或确实缺少的验证。
archive timestamp 不能替代这些证据。

## 文档类别

| 路径 | 状态 | 能否证明当前行为 |
| --- | --- | --- |
| `AGENTS.md`、嵌套 `AGENTS.md` | Operational | 只能约束工作方式，不能证明实现。 |
| [`development-runbook.md`](development-runbook.md) | Operational | 路由当前命令；列出命令不等于该命令当前通过。 |
| [`knowledge-base.md`](knowledge-base.md) | Operational | 约束 GitHub → Notion → Drive 的有界记录；外部知识与交付物不证明当前实现或验收。 |
| [`docs/analytix/specs/README.md`](specs/README.md) 与已登记 specs | Registry + accepted targets/contracts + historical reference | 登记表决定 lifecycle；accepted 文档能定义目标，当前实现仍需代码/测试证据。 |
| `openspec/changes/<active-change>/` | Scoped proposal/work plan | 只有当前请求采纳的 scope 才能指导施工；未完成任务不能写成现状。 |
| `openspec/changes/archive/` | Accepted decision/history | 可解释已归档 change 的决策，不自动证明当前 worktree 仍通过。 |
| `openspec/specs/` | Accepted scoped requirements | 定义已接受的 scoped requirement；实现与 currentness 仍需代码和验证证明。 |
| [`history-rewrite-provenance-2026-07-10.json`](history-rewrite-provenance-2026-07-10.json) | Local Git provenance | 只解析被分类为 Analytix-local 的旧 commit identity；不改写 upstream SHA，也不证明旧 evidence 当前有效。 |
| `docs/analytix/upstreams/` | Reference / chronological ledger | 用于来源、取舍和阶段记录；旧 gate 状态不能覆盖当前代码。 |
| `docs/analytix/benchmarks/` | Benchmark definition + dated results | 定义可复跑标准；旧 score 需要按当前 commit 重跑。 |
| [`docs/analytix/qa/windows-qa-operator-runbook.md`](qa/windows-qa-operator-runbook.md) | Operational | 定义无密 Windows QA 操作方法；不是某次 QA pass 证据。 |
| `docs/analytix/qa/` 中其余报告、`validation-evidence/` | Dated evidence | 只对记录的 commit、平台、环境和命令有效；详见 [`qa/README.md`](qa/README.md)。 |
| `docs/legacy/`、`release/legacy/` | Historical | 只用于迁移、考古和回归背景。 |
| `output/` | Generated/local result snapshot | 不是源码或规范；不得从这里反向修改主实现。 |
| `dist/`、`dist-*`、打包 resources | Generated/package snapshot | 不编辑；内容可能落后于源码。 |
| `vendor/` | Vendored/package input | 不是日常开发入口，除非任务明确要求更新 vendor。 |
| `.kunsdd/` | 用户工作流/兼容格式数据 | 不是 analytix 架构规范。 |

2026-07-10 在增加本索引前的一次盘点发现 634 个受 Git 跟踪的 Markdown，
其中 375 个位于 `output/`、`dist-standard-win/` 或 `vendor/`。这个数字只是
当日审计快照，但足以说明：全文搜索结果必须先按上表分类，不能直接按命中内容
施工。

## 证据新鲜度

只有同时满足下列条件，旧报告才可以支持当前时态结论：

1. 记录了被测 commit 或可还原 worktree 状态；
2. 命令仍存在且语义未改变；
3. 被验证源码内容、命令语义、相关平台/环境和覆盖范围仍与当前结论匹配；
   变化导致证据失效或缺少必要覆盖时，执行相应的新验证；
4. 输出没有依赖已轮换凭据、已删除 fixture、旧打包物或人工未复核状态；
5. 结果明确区分 `pass`、`partial`、`skipped`、`blocked` 与
   `not_configured`；
6. 没有把 fixture/conformance proof 提升成 live provider、真实 GUI、真实
   MCP 或 release authorization。

`generatedAt` 较新、文件名带 `final`、旧报告写着 `passed`，都不能替代以上
条件。`sourceCommit` 不同须核对相关内容和环境；内容相同的本地集成本身不使
证据失效。正式包仍须绑定其要求的精确 artifact，源码或组件证据不能代替它。

2026-07-10 的受控本地历史重写改变了部分 Analytix commit identity。历史报告中
记录的 pre-rewrite `sourceCommit` 保留原值，使用
[`history-rewrite-provenance-2026-07-10.json`](history-rewrite-provenance-2026-07-10.json)
解析到 retained commit；current machine manifest 使用 post-rewrite SHA。该映射
只适用于文件中明确列出的 Analytix-local identifiers，绝不能用于机械替换 Kun、
Reasonix 或其他 upstream commit/tag/blob SHA。映射只能恢复对象定位，不能把旧
报告升级为 current pass；当前结论仍需 fresh rerun。

OpenSpec 也遵守同一条规则：`openspec/specs/` 是已接受 requirement，active
change 是 scoped target，archive 是决策与施工历史。三者都不能单独证明当前
代码、当前 package 或当前外部环境通过。归档 change 后若实现继续演进，当前
结论仍须回到代码、适用的 focused 证据和确有必要的新验证。

## 新文档的状态头

新的关键规范、runbook 或证据文档应在标题后用简短字段说明生命周期：

```text
Status: Normative | Operational | Reference | Historical
Applies to: <scope>
Current as of: <commit/date, only when meaningful>
Source of truth: <code/spec/command>
Supersedes / Superseded by: <path, when applicable>
```

不要为了形式给所有旧文档批量加 frontmatter。优先处理会被 Agent 当作当前入口
的高风险文档；路径分类已经明确的历史/生成目录无需逐文件重复标记。

## 修改文档时的规则

- 先验证路径、命令、分支名、runtime ownership 和配置字段是否仍存在。
- 使用 repo-relative 链接；本机绝对路径只允许出现在明确的历史来源、provenance
  或限定作用域的 host-configuration 文档中。
- 当前行为、目标行为和历史行为分段写，不在同一段落混用时态。
- 中英文文档描述同一事实时同步更新，避免一份成为隐性旧规范。
- 旧证据若仍有审计价值，优先加 currentness/historical banner，而不是改写其
  原始结果；明确含密或误导风险的内容例外，必须清理。
- 不在 Markdown、fixture、日志或示例中保存真实密码、private key、product
  key、remote-control ID、API key、token 或恢复码。
- 文档改动至少运行 `git diff --check`，并检查新增链接和引用路径；只有在
  present-tense claim 依赖可执行事实时，才补跑对应的最小测试/构建/命令。

## 2026-07-10 审计后续

以下是当时的历史记录，不代表当前 remote、跟踪文件或验收状态。
当前已配置 GitHub `origin/main`；`managed-chrome/` 和部分 native 资源已排除
于公开树，现状及准备方法见 [Development baseline](development-baseline.md)。

以下事项最初由文档审计发现，随后通过独立 owner、OpenSpec scope 和验证完成，
或保留为明确的外部边界：

- 当前 Windows 操作入口已经替换为
  [`qa/windows-qa-operator-runbook.md`](qa/windows-qa-operator-runbook.md)，只含
  批准的非秘密 host/account/port、SSH alias 和本地 secret-manager locator。
  2026-07-10 已完成 operator credential rotation、SSH key 登录验证、本地全部
  retained refs 的历史清理、不可达对象回收与临时恢复 bundle 删除；本 checkout
  未配置 Git remote。任何由其他 owner 独立保存的重写前 clone、bundle、备份或
  release archive 均不在本地闭环边界内，未经同等清理与验证不得重新导入对象。
- 另一份已删除但仍在历史中包含个人金融标识的报告也已从全部本地 retained
  refs 和对象库清除；验证只记录路径/对象计数，不回显内容。两次 history rewrite
  引起的 Analytix-local SHA 变化由上述 provenance map 解析，external upstream
  SHA 保持原值。
- execution-policy version-2 safe fresh defaults、exact legacy one-time
  migration、unattended policy 与 Go host-shell restriction 已落地并通过 focused
  TypeScript/Go tests。后续 currentness 仍须以当前 worktree fresh rerun 为准；
  当前文档必须区分应用层 tool/path policy 与操作系统沙箱。
- TypeScript compatibility schema 仍接受若干 Go production 未消费的配置；
  runtime projection 与当前 UI 已不再把已知的 storage/hook/token/compaction、
  tool tuning、memory enable 或 Go media-generation 字段宣传为 active。以
  `docs/ANALYTIX_CONFIG.md` 的 as-built matrix 为准，不能从“字段可解析或保存”
  推断功能生效。
- ESLint 已将 agent-tool metadata、generated output/evidence、managed browser、
  vendor 和 `dist*` payload 排除在 maintained-source gate 之外。当前 lint 结论
  仍须以本 worktree 的 `npm run lint` fresh rerun 为准。
- root `output/`、`dist-standard-win*`、`validation-evidence/` 与
  `packages/runtime-go/output/` 已从 Git index 移除并 ignore，本机生成物保留；
  新鲜 clone 不再把它们当作 maintained source。`vendor/` 与
  `managed-chrome/` 是 required packaging inputs，仍保持跟踪，只按职责排除
  lint/search。
- `hub-account-startup-auth` 与 `vision-bridge-text-model-image-input` 已通过正式
  archive workflow，同步后的 accepted requirements 位于 `openspec/specs/`。
- `close-security-config-artifact-risks` 也已完成 archive/sync；安全执行默认值、
  production config truthfulness、repository evidence hygiene 与 Windows QA
  operator access 四项 accepted scoped requirements 位于 `openspec/specs/`。
  archived change 只保留决策与施工历史，当前实现结论仍须查代码和 fresh rerun。
  其余 active changes 必须通过 `npx openspec list --json` 动态确认，并以各自
  `tasks.md` 判断施工进度，不以 artifact 已生成代替完成状态。
