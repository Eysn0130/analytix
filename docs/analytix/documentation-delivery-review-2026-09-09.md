# Analytix 文档、OpenSpec 与交付推进审阅

Status: Reference — 2026-09-09 的审阅快照，不是新的施工总纲或发布证明。

Owner currentness update：R129 已于本次审阅后的安全窗口集成为
`a738be9d950d2e515a63c26e7253653405567c38`，F1/F2 focused accepted，
659/660/661 证据按相同源码与实际覆盖复用。下文关于 W3、未提交候选及待 review
的描述均保留为原审阅时点，不代表当前状态。当前有限缺口和接续入口见
[任务状态](../../openspec/changes/case-evidence-publication-gate/tasks.md)；
full9.8a、A0/B1、Product/Package/Formal RC 尚未因此完成。

本轮 Owner 定向源码复核另确认：InitialSetupDialog 使用组件内临时草稿，经专用
typed credential-write 提交；Base64 可逆，并非无密传输。main 的严格响应校验
与原文/编码回显拦截、成功后的草稿重置以及 key-free settings 广播均存在。
未发现该链把凭据送入共享或持久 renderer store 的证据；这不是全产品零泄漏
证明，也未放宽 accepted spec 的禁令。输入生命周期负例与规范措辞边界仍在
任务状态中单列。9.11 的操作句已内联改为正常本地 Provider 路径；原审阅所记
未修改 tasks 的事实保留。双语 runtime README 和证据复用规则一并纠偏。

范围：仓库文档与工作指南、三份指定 OpenSpec、主线程当前交付路径。
基线：canonical `HEAD cb3c9cbe5c27b6722e9ceb60a8163808225ca926` 的脏工作区。
产品施工仍由主线程负责；本次仅修改文档，不修改产品代码、验收勾选、
Controller 状态、writer lease、Git index 或 HEAD。

## 结论

延迟不能简单归因于模型推理档位、OpenSpec 或测试过多。可直接确认的主要问题是：

1. 本地 Provider 迁移已经进入 accepted specs，但 README、AGENTS、配置与
   QA 入口仍在指挥 Hub-first 启动。按两套指令同时验收，会制造不可能的目标。
2. 当前 9.8a 涉及可选专业域与共享启动/持久化恢复的实际耦合。它不是关闭一个
   Funds 开关即可完成；既有 protected history 也不能因为普通启动需要成功而
   被丢弃、修复成另一种事实或降格为普通历史。
3. 一些验证迭代先暴露测试构造/编译问题，再触及产品缺陷；候选不断增长，却
   尚未形成完整里程碑验收。这是收敛方式的风险，不等于每条安全测试都多余。
4. 历史流水挤占当前计划，跨 change 的同一义务缺少清楚的复用关系；上层任务
   数和局部 PASS 数都不能用于估算完成比例。

此前把某个局部失败视为长期不变的剩余阻塞，或给出数周交付估计，都缺乏足够
依据。本次不沿用这些估计，也不把命令数当作耗时占比。

## 审阅覆盖与限制

- 以 `git ls-files` 盘点 364 个现存 tracked Markdown/MDX：docs 130、
  OpenSpec 88、plugins 85，其余为指南、工作流、包文档和历史/第三方材料。
  数量是修订前的时点快照，不纳入生成缓存与外部 vault。
- 对全体文件执行生命周期/旧术语检索与简单相对 Markdown 链接检查；
  对六份 AGENTS、当前路由文档、三份 change 的任务与需求结构进行审阅，
  对高影响冲突读取正文并对照规范及生产调用点。
- 三份 change 共 29 个 Markdown 工件；逐 capability 对照 active/archive
  delta 与 main spec。case 的 architecture、migration、threat model、test
  strategy、rollout 也属于依赖材料，不能只检查四个标准工件的存在。
- 这不是对全部数兆字节历史正文的逐句真实性认证，也不是代码安全审计。
  自动链接检查不证明远程 URL、Markdown anchor 或历史命令今天仍然有效。
- 没有运行产品 Go/Electron/live Provider/package 验收。下述施工状态引用现有
  终端记录，独立核验的是文档、源文件和证据记录的内容，而非复跑全部结果。

## 三份 OpenSpec 的逐项结论

### establish-local-provider-credential-authority

实际位置是
[`archive/2026-08-31-establish-local-provider-credential-authority`](../../openspec/changes/archive/2026-08-31-establish-local-provider-credential-authority/proposal.md)，
不再是 active change。61 个任务均已勾选；这是归档记录，不是今天再次验证过。

- **需求同步：** 本地凭据 capability 的 11 个 requirement 与归档 delta 对应；
  四个 `hub-*` capability 的保留新要求与 main 对应，旧 Hub-first 要求属于
  delta 的 REMOVED 部分。不能因文件仍叫 `hub-*` 就继续执行旧登录流程。
- **当前源码观察：**
  [local-provider-readiness.ts](../../src/renderer/src/account/local-provider-readiness.ts)
  按 Registry 连接与凭据配置区分 setup/ready/recovery；
  [main/index.ts](../../src/main/index.ts) 将 Hub account service 放在显式 lazy
  loader 后。上述观察支持文档纠偏，但不替代完整 Hub-zero 运行证明。
- **已修正：** 根指南、双语 README/DESIGN、配置指南、runtime README、Windows
  QA 的过时 Hub 默认与验收方法；五份相关 accepted spec 的 Purpose 占位文案。
  归档文件正文与历史验收结果未改写。
- **仍需核实：** 规范要求 renderer state / public IPC 无凭据，
  [InitialSetupDialog.tsx](../../src/renderer/src/components/InitialSetupDialog.tsx)
  的 `selectedDraft.apiKey`、`setDrafts` 和受控 input 明确让新输入的密钥进入
  React 草稿。要区分短暂受控输入、通用 renderer store、持久快照、公开读取
  与专用写入通道，再检查清理和泄漏边界。当前证据不足以声称已发生泄漏，
  也不足以宣称字面上的“renderer state 零凭据”。本次没有放宽该安全要求。
- **交付边界：** K10 的 fresh local-nonpublishable package 证据不能升级为
  Formal RC；不应重开 K1–K10 全套开发，除非有当前缺陷或证据失效的具体范围。

### case-evidence-publication-gate

这是 active change。任务结构为 115 checked / 81 unchecked；勾选计数不是
产品完成率。其 [convergence map](../../openspec/changes/case-evidence-publication-gate/tasks.md)
已经允许分别记录 A0 普通 Agent 与 B1 有价值资金分析。

- **目标有效：** 禁止伪造案件事实；高风险事实的发布必须经过 Host 证据门；
  普通工作与案件能力共用一个 Agent，案件不可用仅影响相关 protected effect。
  不通过降低断言、恢复自由事实输出或吞掉共享存储错误来加速。
- **具体矛盾：** task 9.11 的 Hub 登录方法与已接受本地 Provider 规范冲突。
  本次在 proposal 和根指南声明同 scope 的替代关系。tasks.md 正由主线程追加，
  保留其原字节与勾选；主线程在安全编辑边界应将该句内联改为正常本地 Provider
  onboarding / Settings，保留其他 source/model/artifact/隐私要求。
- **另一处纠偏：** rollout 曾把“current signed general-risk policy”写成
  普通可用性的笼统前置条件。现在明确普通工作遵循自身 Core policy/grants，
  可选案件侧故障不阻塞独立普通工作；已有 protected lineage 不得据此降格。
  依据是 accepted kernel 的 `Case Evidence Is Optional To General Agent Work`
  与 desktop spec 的 `Production Gate Is Mandatory`，不是新减配决定。
- **同步状态：** 部分数据取证、typed local display、Provider context、release
  safety 与 source-readiness 的 active delta 比 main spec 更新。此前 registry
  的“已同步”只覆盖 2026-08-16 快照，本次明确其边界；没有借审阅批量同步未完成
  变更或伪造实现接受。
- **收敛风险：** tasks.md 已混入大量执行流水，9.8a 的新结果继续追加。
  应在当前 writer 的安全边界保留可追溯历史、提炼当前剩余义务和最新证据引用。
  不逐文件创建新 Slice，不因一行旧任务存在就重做已经满足的同一义务。

### define-agent-platform-brand-architecture

这是 active change，14 checked / 16 unchecked，不能因 CLI
`isPlanningComplete=true`、`isComplete=true` 就称架构交付完成。

- **保留方向：** 一个 Go Harness、内置不可绕过的 Privacy Layer、首方静态插件；
  Funds 退出通用启动依赖。没有证据支持为加快交付再建第二 runtime 或 authority。
- **具体纠偏：** 插件 AGENTS 原来只要求目录与 `.codex-plugin/plugin.json` 的
  name 一致。当前自有声明/已实现 package 工作还要求 canonical identity/version、
  平台投影一致、Host admission 与 grant 分离；本次改正指南的错误归属。
- **重复义务：** 2.4、4.3、4.4 与 case 9.8a 的隔离工作共享实现范围；3.x 与
  case privacy seam 也有重合。proposal 现在明确按同一义务复用有效证据，
  不把两个任务编号误当两个工程。
- **边界：** 2.2a 本地 native admission、2.3 registry、剩余 Privacy Layer、
  迁移与完整平台验收仍有未完成工作。2.5 是第三方 executable admission 的
  前置条件；未开放该能力不能自动变成既定首方 A0/B1 交付的前置施工。
  这不勾掉平台任务，也不等于整份平台 change 可以提前归档。
- **同步风险：** 后增 canonical declaration 和 local-build 要求尚不全部在 main
  foundation spec。施工应识别当前已授权 delta，归档时按 canonical lifecycle
  同步；不能通过只读旧 main 或只看完成标题来遗漏要求。

## 主线程为什么仍未交付

2026-09-09 读取 Controller 状态时，writer 为
`AI-R129-W3/rev11 / LEGACY_REPOSITORY_EXCLUSIVE`，A–D 与 full 9.8a 仍 OPEN。
`HEAD` 最近三次提交为文档/治理记录；当前产品候选仍在工作区，不能以文档提交
次数代表产品集成进展。本次未取得写租约转移，也未改动共享 index/HEAD。

源代码 [runtimeapp/app.go](../../packages/runtime-go/internal/runtimeapp/app.go)
将 optional installation、child identity、original registry、authority advance、
report preservation 和 semantic recovery 串入启动。部分错误直接返回全局失败。
因此每修复一类历史状态，都可能继续碰到下一层共享恢复前置条件。问题是隔离边界
与持久状态之间的耦合，不是测试文件数量本身。

本次读取的 R129 现有终端记录给出以下具体例子：

| 记录 | 观察 | 对推进的含义 |
| --- | --- | --- |
| 600 | native intent 已落盘、witness 前中止；普通启动被全局 gate 拦住，RED | 是实际恢复/隔离缺口，不能当无用极端测试删除 |
| 601 | 新测试的 unused `ctx` 导致编译失败 | 没有检验产品行为，属于可避免的测试准备成本 |
| 602 | unsettled-intent preservation 终端 PASS | 仅关闭该定向范围，不能推导 9.8a 全部完成 |
| 603 | portability 终端 PASS | 不等于目标 Windows 文件系统上的运行验收 |
| 604 / 605 | 分别 BUILD_FAILED / FIXTURE_FAILED | 又花两次尝试才获得有效缺陷信号，说明测试构造需要收敛 |
| 606 | native UNKNOWN committed 场景 RED | 是后续产品缺口；不能把较早 555 当成仍未变化的唯一问题 |

记录位于本机 Controller 的 `r129-evidence`，这里只保留编号和脱敏结果。
这些是读取已有记录所得；本次未独立复跑。主线程继续运行，最后一个编号不保证
仍是用户阅读时的最新状态。

交付前补查的 Controller 快照更新于 2026-09-09 10:40:48（Asia/Shanghai）：
下一项已推进为同 installation 下 closed-completed 与 open report 的混合历史，
要求保留完整 master/orphan inventory、只 hold 未解决的 thread，并核对两个
route 分支；状态仍为 `R129_IMPLEMENTATION`、A–D/full 9.8a OPEN。
该快照已明确 Superseded 没有当前 Coordinator producer，不制造原生生产声明。
这说明主线程也在主动收敛，不能把较早测试失误概括成“所有后续工作都无用”。

**可减少的工作**是重复装载长上下文、过期状态再确认、没有新覆盖的全套重跑、
同一义务在两份 change 中再次施工，以及没有 production producer 的未来状态
扩展。**不能削减的工作**是实际持久化恢复、unknown 不升级、已提交事实保全、
共享 Core 损坏的 fail-closed、隐私出口和真实打包公共缝验收。

## 保持质量的交付路径

以下是对既有目标的执行建议，不新增 release gate 或未经授权的产品范围。

1. 当前 writer 在下一安全边界冻结 9.8a 的剩余义务，以真实 producer、可达状态、
   restart consumer 与预期公共行为组织。每个新增项说明它关闭哪条 accepted
   invariant；领域枚举里存在但生产不可达的未来状态不自动扩成产品功能。
2. 把同一根因的一组修复完成到公共边界后再验收。对等价输入在最窄的 owner
   验证规则，公开入口验证传播；只有威胁模型或层间差异需要时才重复完整组合。
   这不是省略已知失败，也不设置任意次数上限。
3. 证据按候选与影响范围复用；无源变化、无环境变化、覆盖相同的检查无需机械
   重跑。最终集成候选仍须做与变更风险相称的 fresh 关键验证。
4. 关闭局部隔离缺口后，尽快交付可审查的 canonical 产品提交与 A0 独立结果。
   A0/B1 共用同一 Agent；B1 仍需真正的 `analyze_account_flows` 和 typed local
   display，不能以 count canary 代替。当前任务的正式包、模型与运行权限照旧。
5. live Provider 或外部条件缺失，只阻塞依赖它的验收行；继续可独立完成的
   B1 deterministic 工作。完整首阶段交付必须分别满足 A0 与 B1，不把 A0 当全交付。
6. 后续第三方执行、泛化报告/authority 扩展与未要求的平台不得从历史队列自动
   回流当前首阶段；发现确实影响正确性/安全的关联缺陷则保留在必要修复范围。

当前没有足够数据给出可靠日期。可确认的是：主线程尚未完成共同的启动隔离
义务，后面还有集成与 A0/B1 验收。只有剩余义务稳定、关键 public seam 实测后，
才有条件给分阶段估计，而不是用几百个命令或任务勾选比例换算工期。

## Skills 与历史文档的处置

- `analytix-rc-control` 已写明有限交付、按差异恢复、证据复用、不强制微切片与
  模型降级。没有证据证明需要重写整套 RC Skill。现有运行记录与这些原则不完全
  一致时，应改具体执行与当前状态记录，不再新增一层控制协议。
- `openspec-apply-change` 原来要求读取所有 `contextFiles`，没有明确同会话复用。
  本次补充只重载变化/已失去上下文的工件，并区分 planning complete 与产品完成；
  共用义务按实际候选和覆盖复用证据。没有降低需求阅读或验收标准。
- 上游 OpenSpec 把 workflow 视为可按依赖选择的动作，而非锁死阶段；这支持
  修改已有计划并继续，不支持把每个小修复变成一轮提案。
  [官方工作流](https://github.com/Fission-AI/OpenSpec/blob/main/docs/workflows.md)。
- Claude Code 官方建议提供可执行验证信号、按复杂度规划，并提醒上下文膨胀
  的成本；不能据此推断 Astra 的具体速度、推理档位或本项目耗时比例。
  [官方实践](https://code.claude.com/docs/en/best-practices)。
  本次通过 Exa 查阅这些公开资料，只比较行为原则，没有复制上游实现或技能。
- 旧 QA、归档 change、上游 ledger 和插件 reference 镜像具有证据或消费价值，
  不作无差别删除。已把文档地图中 2026-07-29 的长比较移到独立 Historical
  reference，保留旧标题入口；修正 consolidation register 把 01–11 全部称为
  Normative 的错误，并明确历史整理队列不阻塞产品交付。

## 本次验证与待办边界

本次执行：

- `git diff --check`：PASS。
- 366 个现存 tracked/untracked Markdown/MDX 的简单相对文件链接检查：
  缺失目标 0；不包含远程 URL 或 anchor 真实性验证。
- 迁出的历史比较正文与原 HEAD 对应段落一致：PASS。
- `npm run verify:openspec-codex-skills`：PASS，canonical skills 6，重复/旧副本 0。
- `skill-creator/scripts/quick_validate.py` 对修改的 apply skill：PASS。
- 两份 active change 的 `openspec validate <name> --type change --strict --json`：PASS。
- `openspec validate --specs --strict --json`：首轮 19/20，唯一失败是原有
  context-epoch Purpose 占位句；补充用途后再次执行为 20/20 PASS。

上述命令在需要时同 shell 加载缓存 helper；未启动产品测试、实时模型调用或打包。
没有为了清除 long-requirement INFO 提示拆解安全要求或增加新任务。
文档保存在 canonical 工作区但未提交；主线程仍持有产品独占写入，本次不改变其
共享 index/HEAD。外部知识库未写入，本次修改范围为 Analytix 仓库文档。
不将文档校验当作产品 PASS。

未修改主线程正在写入的 tasks.md，其 9.11 旧措辞已有 proposal/根指南明确的
替代规则，但内联整理仍应由当前 writer 在安全边界完成。
凭据瞬态输入合同差异仍待有针对性的安全复核；不通过本报告自行关闭。


## Historical appendix — operational snapshots relocated from d469a7406

Historical only. Added 2026-10-02 from the exact source baseline above. Original
candidates, failures, dates and task IDs remain scoped to their recorded environments;
this appendix adds no acceptance or current work. Current routes stay at the source
paths below. Relative Markdown links are rebased; original text remains in Git.

<a id="snapshot-d469-case-tasks-1"></a>

### case-tasks: original lines 1–22

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `5a52ac1ca63528256abdded9a35392fe9b9cc104af4d31cc8c0abbe4e5be0d59`.

## Owner acceptance update — 2026-09-11

The original full9.8a and shared platform 2.4/4.3/4.4 scope is now accepted.
R129's accepted lane isolation and complete Original/UNKNOWN, held-history and
Core protection remain the foundation. SHARED-024 adds all 55 real ordinary
operations across missing, disabled, incompatible, unauthorized and
domain-semantic faults; SHARED-025 passes all 15 public-consumer checks on that
exact corpus, with zero skips and closed task process/FD/fixture cleanup.
Shared rev6's 26 source files are Owner-reviewed. These results fill the
previous checkpoint's remaining ordinary-lifecycle/public-consumer gap.

The Owner closure is SHA256
`6c48003d56208717984e8da38eba990df34da39d2f82fc0e0acd9d8f4e08fc6e`;
the cumulative applicability review is SHA256
`2f9481e54557412a29078463e45429bc4a55c13276a38c2d3c7b1cd7d25ecc14`.
Evidence retains its actual B1rev10/Sharedrev6 source snapshot
`a5ec21fdc675a1eab4acf885d37964a27910713c24e00ef9bb9bd1f94ed6671f`.
Provider peers were synthetic/loopback and Electron transport shells were
simulated; formal Electron, installed/native artifacts, actual A0/B1 and
Product/Formal RC remain separately open. Historical failures and PROCESS004
DP uncertainty are preserved. Earlier open statements below are historical
for these four rows only; requirement text and all other task states remain.

<a id="snapshot-d469-case-tasks-76"></a>

### case-tasks: original lines 76–158

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `ad705bae49c8e09a6a2da3675e54cc05b84fba9302d17e8c884175c80fcef8bd`.

## B1 same-execution focused checkpoint — 2026-09-10

Owner accepted the 40-file candidate
`c32b732dfabfa489d6dcf8d3d7c052d2144d17f42ed91ec60ff263b98475c3f1`
on baseline `5680a528e3a60d7bb8903ca254feca829ad44923`; Slice revision 2
COMPLETE_BRIEF has a verified release. Command 054 passes actual native import,
production settlement/receipts/claims/Final Gate, full/masked original slots,
second-snapshot evolution, current preview, durable handler reopen, retained
missing/corrupt rejection and ordinary synthetic Provider continuity. Partial
row evidence remains a partial answer. Owner activation and budget/legacy
compatibility race checks pass; all prior failures and seven clean-HEAD
reproduced baseline child failures remain recorded.

This establishes current Go composition with fixed historical development
native input only. It does not close entire B1, current-source native package,
actual runtime process, Electron, Windows, performance or Product/Formal RC.
See the [current recovery checkpoint](handovers/2026-09-10-owner-replacement.md)
for implementation boundaries and exact evidence locators. Next establish the
current native generation/runtime-process seam without replaying old one-shot
commands. No task checkbox or acceptance requirement changes here.

## R131 focused checkpoint — 2026-09-10

Owner reviewed the preserved R131 candidate and both terminal-closure revisions.
The 53-file candidate fingerprint is
`501d5ae3c9f2b97156d36d31d1498a960af3835c172f8a2e5070fc3c6c753826` against
HEAD `4b4392dc2d1ae4a0683ef46f71ecd93007acc9bc`. GENERAL next-start settlement
and CASE duplicate private-preparation admission are repaired, with separate
controlled-fault, cancellation, production-restart and public-projection evidence.
Current R131-F1/F1-CASE invariants are focused accepted; historical matrix36 cause
remains UNKNOWN. Authenticated longitudinal semantic corruption remains
degraded/NOT_RECOVERED while preserving the committed final and ordinary work.
See the [current recovery checkpoint](handovers/2026-09-10-owner-replacement.md)
for evidence and claim limits. No task checkbox changes: full9.8a, shared platform
2.4/4.3/4.4 and A0/B1/Product/Formal remain open. The next bundle is the existing
B1 deterministic value/native-entry/local-display gap, not a new R131 dispatch.
The following 2026-09-09 routing remains historical context.

## Current delivery checkpoint — 2026-09-09

R129 is integrated at `a738be9d950d2e515a63c26e7253653405567c38`
(471 task-owned paths). Owner review round3 closed F1 and F2. The unchanged
candidate is bound to R129-659/660/661 and source fingerprint
`6fe2b85c1703f5489081822d8b8caf77a255c4be1a44a9169cb533327c936837`.
659 covers the scoped recovery/history consumers and Windows compile-only;
660 covers the actual compacted derived-history/held-report native fixture,
ordinary HTTP work and two offline starts without Provider/witness replay;
661 records strict planning validation. Their exact scope is retained below.
Earlier "review/integration pending" paragraphs are historical execution
snapshots, not the current delivery state. Full9.8a, A0/B1 and Product/Package/
Formal RC remain open; no task is checked from this focused integration alone.

R130's local Provider acceptance-harness contract is now Owner-reviewed and
integrated with this checkpoint. Its 14-file tested candidate fingerprint is
`b231917b8cb4840f738c79a2cfa4bd19d79ff2d2e74a091239964982978ae980`.
A0/B1 observation, closed reports, formal validation, packaged-QA and request
audit now use normal local Registry/Secret Store authority instead of Hub-first
state. The private leakage needle is bound to the actual visible entry's
completion readback, including Provider/Registry identity and credential
generation; first-ready, recovery and final-scan drift fails closed. No Secret
Store export, credential fingerprint or second authentication authority was added.
Controller final evidence: 723 affected synthetic tests and Node typecheck pass;
Owner independently passed 37 selected same-entry/scan/formal regression cases
(479 unrelated cases unselected). Unchanged audit/aggregate coverage is reused
only within its scope. R130-F1 is closed; its earlier failing counterexample is
preserved. This is focused harness-contract evidence, not a live credential,
packaged A0/B1 or release result; no broader task checkbox changes.

| Current obligation | Classification | Implementation / valid evidence | Exact remaining gap and next action |
| --- | --- | --- | --- |
| Optional domain fault isolation; complete Original/UNKNOWN and held-history authority | 已满足 / satisfied only within the accepted R129 scope | `runtimeapp` optional-domain and preserved-history owners; 659/660/661 above | Do not rework the unchanged graph or treat genuine Core/physical-integrity refusal as a degradable plugin fault. |
| Full9.8a and shared platform 2.4/4.3/4.4 | 验证缺口 / verification gap | R129 plus existing ordinary/case Server public-seam controls | Still prove the complete ordinary Coding/Writing/Research/Skills/MCP/jobs/subagent/compaction/resume lifecycle and desktop consumers across applicable plugin fault classes. A single ordinary read or Windows compile-only does not close this row. |
| Normal local Provider acceptance in A0/B1 | 已满足 / focused harness-contract correction | R130 producer, closed report, formal validator, packaged-QA and request-audit predicates; same-entry/recovery/final-scan authority binding and nonzero private scan | Actual visible onboarding, fixed model/high workflow and current-artifact A0/B1 evidence remain separate. No old artifact/harness pair rerun. |
| B1 local native entry and valuable analysis | 验证缺口 / verification gap | Existing `AuthorityUseLocalBuild`, native Host and Funds lifecycle seams; no second authority is needed | Reconcile the exact-generation 9.10a checkpoint, then prove `analyze_account_flows`, immutable snapshot/Final Gate, Direct Preview and AcceptedSlotDisplay. `count_case_rows` is not B1. |
| Credential-input boundary | 验证缺口 / bounded contract and acceptance gap | Current component-local input uses a dedicated typed credential-write request; main rejects raw/encoded echo and successful setup clears the draft | No shared/persistent renderer-secret leak was established by source review. The absolute renderer-state/public-IPC wording and full input lifetime/negative evidence still need precise closure; no blanket exception is granted. |
| Actual A0/B1 formal evidence | 外部条件或保留授权 / conditions to verify | First-stage convergence map and 9.11/9.11c | Establish the exact current artifact, authorized normal Provider credential and immutable case-input scope, identity/signing conditions and applicable one-run identity before actual execution. Availability has not yet been confirmed; only dependent rows wait, not deterministic B1 work. |

The next implementation bundle is `AI-R131-OPTIONAL-PLUGIN-ORDINARY-LIFECYCLE-20260909`:
complete the ordinary lifecycle and corresponding desktop-consumer evidence
across missing, disabled, incompatible, unauthorized and domain-semantic plugin
faults, reusing valid R129 coverage without reworking the unchanged Original graph.
This table is a current evidence/gap index for the existing tasks, not a new
task denominator or a change to either change's completion requirements.

<a id="snapshot-d469-case-tasks-281"></a>

### case-tasks: original lines 281–644

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `fefa4e489d6cf1e2e51838a6fd1fe4214cd7eb1fad1bb29ac2e3f330cfb0acbe`.

  The revised 2026-08-24 Slice 22 focused source candidate connects the
  enrolled production runtime composition to the existing concrete witnessed
  evidence registry service. `app.go` selects the one
  `sharedEvidenceDatasetSnapshotV2.registry` instance as Registry,
  LockedSnapshot, and FactFinalWitnessIssuer and passes it through
  `NewCasePublicationFinalizerWithHostEvidenceAuthority`; an AST gate forbids
  returning production composition to the compatibility constructor. A real
  concrete service plus real Secure Private CAS V2 index/capsule adapters then
  executes `PersistBoundary`, issues a V2 Fact-Final Witness for the current
  exact registry/DSV2/binding/source snapshot, and reaches the next legal
  `EvidenceBackedAnswer` terminal. Revocation, concurrent registry drift,
  cross-snapshot, and cross-case binding remain boundary-only. The non-empty
  index-to-capsule recovery plan validates and revalidates, and a fresh
  adapter/service instance revalidates the same receipt, current index,
  current capsule, locked snapshot, and existing witness without another
  registry advance or re-signature; the old wrappers remain alive, so this is
  not strict close/reopen or OS-process restart evidence. A synthetic
  production-server seam additionally passes that same concrete service type
  through `NewRuntimeServerHandlerFromComponents`, executes the handler-owned
  publication authority and real finalizer, verifies the issued witness back
  against the concrete registry, and observes the bounded typed final through
  HTTP and exactly one SSE `accepted_final_batch` without a Provider or live
  source call. The complete signed `AcceptedFinalRecord` V5 and every private
  component bound to it—including its embedded V2 core, V2 expansion, witness
  admission, publication proof and digest, registry head, security context,
  publication intent, signature/key material, and private wrapper—remain only
  in host-private durable/CAS or audit authority. No generic HTTP, SSE, event,
  history, IPC, or renderer state serializes that V5 record or any of those
  private components. Existing
  `AcceptedFinalPublicViewV2` keeps its exact version-2 wire shape for strict
  private/legacy comparison; it is not silently repurposed. Current generic
  HTTP history and nested SSE events instead carry the explicit closed
  `AcceptedFinalPublicViewV3`, which retains the accepted PII-free semantic
  fields, `checkedScopeDigest`, masked citation handles, and receipt
  `setDigest`. Its closed wire contains only `schemaVersion=3`,
  `acceptedFinalDigest`, the fixed `publicationState=accepted`, `variant`,
  `terminalReason`, bounded `blockerCode`, `coverageStatus`,
  `checkedScopeDigest`, `missingScopeCount`, `claimCount`, `claimTypes`,
  `receiptMetadata` containing only `projection=masked_metadata_only`, `count`,
  `setDigest`, and citations with `cite_*` handles plus canonical labels,
  `noHitWording`, and
  `acceptedAt`; it does not
  carry `publicViewDigest`, `envelopeIssuedAt`, `threadId`, `turnId`,
  `renderedTextSha256`, or private context bindings. It excludes the
  complete `factFinalWitnessAdmission`, `publicationSnapshotProof` and its
  digest, `securityContext`, `envelope`, `registryHead`, `publicationIntent`,
  `storeDigest`, case/context binding, dataset/source identity, receipt ids and
  digests, raw values, paths, reverse maps, and PII. The durable public event
  manifest is created in that V3 form. `AcceptedFinalDeliveryBatchV1` remains
  a frozen strict private/audit wire with `schemaVersion=1`, purpose
  `analytix.accepted-final-delivery-batch/v1`, and the original private
  accepted-final item event tuple; it neither accepts V3 events nor enters a
  current generic consumer. An exact cross-layer hostile canary keeps the V3
  assistant event unchanged while relabeling only its wrapper as V1; Go and
  TypeScript reject that mixed wire without rewriting it, and Main SSE emits
  no event or cursor advance. Go migration and hydration admit the valid V1 tuple only
  after the evidence owner strictly parses and validates the embedded signed
  V5 record, its complete V2 public-core semantics, and the exact expanded V2
  view; the event owner's raw framing check is never sufficient authority.
  Current Go publication, restart hydration, HTTP,
  SSE, Main, preload, and renderer use the explicit closed
  `AcceptedFinalDeliveryBatchV2` (`schemaVersion=2`, purpose
  `analytix.accepted-final-delivery-batch/v2`) whose assistant event contains
  only `AcceptedFinalPublicViewV3` plus the unchanged error/usage/terminal
  profile. The existing installation-bound `AcceptedFinalDeliverySealV1`
  signs the exact V2 sequenced events and binds the commit, payload manifest
  and batch identifiers; no new seal, key, signer, or delivery authority is
  introduced. Main distinguishes both versioned shapes and admits only V2 to
  the current generic public consumer. A mixed durable history retains each
  frozen V1 and current V2 manifest in its original schema/purpose family;
  response-time hydration never projects or re-seals a private V1 manifest as
  V2. Thread-detail hydration now
  exposes a bounded ordered `acceptedFinalDeliveries` projection containing
  exactly one independently sealed existing-authority batch for every visible
  V3 accepted-final turn/item; the singular `acceptedFinalDelivery` remains
  only as an optional exact single-group compatibility alias and is absent for
  multi-history responses. The HTTP owner clones the public thread projection,
  removes any ambient delivery fields, and requires the rebuilt delivery count
  to equal the visible accepted-final turn count before attaching the array;
  missing or extra groups therefore return no array or naked view. Main verifies
  every batch, event manifest, sequence range, commit, turn, exact assistant
  item, and seal before admitting its corresponding V3 view, and never lets the
  latest attestation authorize an earlier turn. Two-final hostile coverage
  rejects independent replacement of either old or new view, item, batch,
  sequence, commit, or seal, plus missing/duplicate delivery groups, a split
  commit, a repeated turn under a different commit, reversed group order, and
  lower or advanced `latestSeq`; Go separately binds `latestSeq` to the
  complete durable replay frontier while each batch range remains ordered and
  bounded by that frontier. The public multi-history projection is capped at
  256 visible delivery groups. The 257th group fails before any public
  projection or `SealAcceptedFinalDelivery` call; no prefix delivery is
  returned. HTTP rejects an over-bound hydration result without attaching a
  delivery, and SSE performs a full repeat/split/turn/frontier/count preflight
  before issuing the first seal. Exact boundary coverage admits 256 groups and
  rejects 257 in the Go hydration/SSE/HTTP and TypeScript response contracts.
  GET hydration never derives a commit or display authority
  from a standalone V3 object: it first binds each visible slot to the
  host-private V5 record, then requires the matching exact durable manifest
  and independently validates its Batch V2 plus existing DeliverySealV1 before
  returning that V3 slot.
  Existing SSE batches remain independently sealed per publication. This
  reuses the one delivery authority rather than treating V3 or a response-time
  digest as a second authority. Go and TypeScript implement the same strict V3
  shape; the Go Batch V2 owner revalidates that exact closed shape before the
  existing delivery authority seals a durable or projected event, so extra
  private V2/V5 fields cannot become a self-consistent public manifest.
  The V3 view, assistant-item, and item-event parsers validate shape only;
  generic `TurnItem`, `TurnSchema`, and `RuntimeEvent` reject those standalone
  shapes. The renderer's generic item mapper likewise rejects every standalone V3 object, and
  GET history reconstructs an accepted assistant only from that turn's
  independently verified Batch V2 group while retaining its local delivery
  receipt. Generic Main/renderer GET and SSE consumers accept only V3 items, while the
  full V5+V2 schema and verifier remain private/CAS or audit-only. Generic
  Go/TypeScript/Main replay canaries inject the complete Go-issued fact-bearing V5 record,
  including its V2 Fact-Final Witness admission and publication-snapshot proof
  digest, and reject it before generic event sealing, SSE delivery, or renderer
  IPC; detached witness/proof fragments are rejected as well. The positive
  control remains the independently delivery-sealed V3 projection, not a
  field-deleted private record or a silently rewritten V2 wire shape.
  Private-authority detection keeps `acceptedFinal`,
  `factFinalWitnessAdmission`, `publicationSnapshotProof`, and its digest as
  unconditional hostile markers, and recognizes a complete private V5 only by
  its closed structure. Ordinary payloads may use generic names such as
  `envelope`, `registryHead`, `publicationIntent`, or `storeDigest` without
  being misclassified when that V5 structure and every strong marker are
  absent; complete records and malformed/detached strong fragments still fail
  closed. Real ordinary lifecycle and tool events carrying those generic names
  remain eligible for their ordinary projectors; the lifecycle extension is
  omitted and the existing metadata-only tool-arguments projection withholds
  every raw name and value, so detector precision does not widen the generic
  public schema.
  Thread-detail hostile coverage additionally returns a fixed 502 with
  zero reflected bytes for duplicate same-ID assistant/terminal items,
  `privateDiagnostic`, long AuthorityRef, and phone-PII canaries.
  Live SSE likewise rejects a standalone shape-valid V3 item before the public
  projector; only a complete `accepted_final_batch` whose existing delivery
  seal validates may carry nested V3 events, so an identity projector or test
  double cannot substitute for delivery authority.
  Private Main verification validates historical V1
  and newly issued V2 Fact-Final Witness admissions as strict versioned shapes;
  historical V4 accepted finals admit only V1, while live V5 retains
  Go-compatible strict V1/V2 parsing. V2 binds its DSV2 id to the manifest
  digest, checks registry and dataset generation bounds, and requires every
  dataset-root, selected-snapshot, and funds producer-content field. Property-
  name and exact-value canaries cover GET and SSE, while exact positive checks
  retain the V3 `acceptedFinalDigest`, semantic variant, checked
  scope, masked citation handle, and set digest. The existing SSE delivery seal
  remains the transport integrity authority; no new registry, CAS, database,
  public signer, or batch authority is introduced. This focused seam still has
  not crossed the real Funds HTTP plugin/model entry and does not close broad
  product, display, package, Electron, Provider, formal, or matrix rows.

  A genuinely absent evidence-registry owner remains absent across ordinary
  `NewRuntimeServerHandlerE` startup, health, durable turn persistence, and
  handler recreation: production constructs no registry Service, opens no V2
  CAS leaf, initializes no witnessed head, and creates no registry root, lock,
  indexes, capsules, or legacy family. A pre-existing complete generation-zero
  empty pair is physical state, but remains frozen and unavailable rather than
  being advertised as initialized; a direct composition seam also proves both
  cold absence and the empty pair return no concrete registry Service before
  either writable V2 CAS constructor. A persisted ordinary/general turn survives
  configured-V2 handler recreation without registry authority; inventory skips
  it only when no indexed authority uses that turn identity, and rejects any
  collision.

  Generic orphan candidate deletion defers only the evidence-registry owner's
  physical contents; the owner remains one of all 19 runtime V4 recovery
  participants and one of the 15 physical topology owner groups, and its two
  leaves remain inside the complete 56-root authority/topology manifest and
  revalidation denominator. The owner-specific bounded,
  handle-relative physical plan recursively freezes the
  fixed V1 family, partial/current leaves, safe regular residue including
  `.registry-authority-index-*.tmp`, and safe legacy `thread-*` directories
  without deletion, content interpretation, repair, or V2 promotion. Direct
  and nested regular contents are SHA-256 bound; same-size rewrites fail
  revalidation. Symlink, special-file, hard-link, case-alias, count/byte
  overflow, owner/root replacement, and signed journal/manifest conflicts
  remain fatal structural blockers. The reserved lock name gives no exemption:
  lock/root/ancestor/child symlink or special-file inventory fails, and a
  same-bytes owner replacement between the physical probe and composition is
  rejected by revalidation. Strict global snapshots treat the two V2
  CAS leaves as opaque bytes; only the evidence-registry owner parses their
  semantics. Owner recovery and current inventory both require every
  per-thread/turn lineage to begin at `RegistrySequence=1` and every later
  signed entry to be an exact old-to-new extension; signed first-sequence and
  extension gaps remain unavailable. Canonical non-JSON partial or complete leaves therefore leave
  ordinary startup and health available with exact tree bytes/identity frozen,
  while the case capability remains unavailable and creates no missing sibling
  or alternate family. Staged, committed, malformed transaction residue or
  non-registry residue stays fatal.

  V4 carries that optional freeze on each prepared plan, not only on the
  participant's public target list. Stage, pre-witness rollback, commit-marker,
  committed cleanup/finalization, committed-inventory validation, and generation
  retirement all skip the frozen evidence-registry plans while continuing to
  revalidate their physical/topology bindings. A mixed transaction with frozen
  evidence-registry residue and a non-frozen accepted-final residue converges
  only the latter and retires its signed journal; an injected sibling stage
  failure preserves the registry whole-tree digest and identities through
  rollback/retry, leaves no frozen staged/committed phase, and still converges
  the non-frozen participant. Frozen staged or committed registry residue
  remains fatal rather than being rolled back or committed.

  With no shared enrollment, one signed, durable-context-bound non-empty V1
  receipt survives handler restart and remains resolvable/replayable by the
  existing legacy store. When shared V2 is configured over V1 data, or when a
  partial leaf, corrupt V2 record, non-empty lock, or safe unknown residue is
  present, ordinary handler startup and health continue while the optional
  case evidence capability is unavailable. Empty owner roots, either single
  leaf, an empty pair, zero/non-empty lock, and safe unknown residue all count
  as existing state for installation-key preflight; empty and non-empty safe
  regular residue do as well. If the installation key is absent they remain
  blocked and the key is not reconstructed. Only a truly absent owner reports
  no state. The safe unknown-residue seam also drives a
  real ordinary Provider attempt and persisted ordinary restart. On
  that same blocked state, the host-private status creates no persistent
  authority: production does not fall back to legacy, manufacture a missing
  leaf, expose a fake empty inventory, repair or commit registry state,
  advertise case evidence tools, or call the Provider or witness for the case
  request. The case finalizer emits the existing fixed typed boundary. A
  current zero-fact V5 final remains a normal committed, terminal-complete
  public hydration authority after restart when its installation signature,
  exact primary CAS/terminal chain, and bounded public-view binding validate;
  registry unavailability does not turn that source-unavailable or typed
  ordinary result into audit-only state. Its frozen registry head may be a
  historical non-empty head, but is not replayed, reinterpreted, upgraded, or
  used to authorize a fact. Focused preflight plus terminal-restart coverage
  now keeps SourceUnavailable, NeedsEvidence with a typed OrdinaryResult, and
  GeneralGuidance with a typed OrdinaryResult on the normal committed and
  terminal-complete path even when the frozen signed registry head is
  historically non-empty and current registry replay is unavailable.
  Configured V2 never falls
  back to the V1 store; legacy activation remains available only when shared V2
  enrollment is genuinely not configured. Terminal recovery has no
  registry-unavailable exception that can downgrade an executable current V5
  record: placing any such zero-fact record in audit-only inventory is rejected
  across marker-false/marker-true and empty/non-empty historical registry-head
  fixtures, regardless of blocker or answer variant. Audit validation and installation
  trust always run for genuinely non-executable inventory. Fact-bearing current V5
  and historical V2/V3/V4 records remain on the existing non-executable audit
  path while registry replay/proof/witness authority is unavailable and cause
  no recovery writes. Ordinary thread listing and detail hydration retain their
  existing behavior; no list skip or fixed 503 substitutes for validating a
  current zero-fact final. Safe unknown V1 residue is
  physically frozen only and is never semantically recovered or promoted.
  Strict non-empty OS-process restart and a real Funds HTTP plugin ingress
  remain for the final source/Electron matrix. This focused source composition did not
  traverse a real Funds HTTP plugin ingress and ran no Electron, package,
  Provider, or formal evidence. It does not close 3.13, 3.17, 3.19, 5.6, 5.7,
  5.11, 5.12, 6.10, 7.1b, 7.2, 7.3, 9.5, 9.11, 9.11c, 10.4, or 10.7-10.9;
  all remain unchecked.

  The 2026-08-23 Slice 10 focused candidate closes the identified exact
  auto-continue and startup-recovery crash cuts without adding a store,
  registry, grant, protocol, provider path, or parent re-finalization path.
  Auto-continue notices are rebuilt only from the durable child record and its
  stable `autoContinueUpdatedAt`, then use the existing one-lock exact
  background-item ensure; a durable closed status with a missing notice is
  repaired without re-running the gate or Provider. Recovered parent tool
  settlement now revalidates the latest parent/security/tool identity under
  the durable thread mutex and applies the tool-call status plus the exact
  tool-result in one cloned mutation, one primary write, and exact readback.
  Recovered result timestamps come from the durable recovery record. Exact
  replay has no event side effect, while same-id/different-byte, unsafe-parent,
  and accepted-final mutations fail closed.

  Startup reconciliation also repairs a missing settled
  delivered/skipped/dead-letter delivery-ledger projection directly from the
  durable record without changing its status, attempts, or timestamps and
  without replaying lifecycle or Provider work. OBSERVED synthetic focused and
  race checks cover 32-way live/recovery auto notice convergence, 16-way
  interrupted recovery, 32-way atomic parent settlement, patch-only and
  acknowledged-lost crash cuts, all three settled delivery states, exact
  conflict no-overwrite/no-event behavior, and hostile caller PII/long-ref/path
  withholding. These source tests are not the wider RC, package, Electron,
  Provider, or formal acceptance matrix, so 5.6 and 5.7 remain unchecked.

  The same-day Slice 10 P1 append follow-up makes both startup parent-identity
  admission and the durable-mutex recovery blocker reject a parent whose
  `status` is `archived` or whose legacy `archived` flag is true with the
  bounded `parent_thread_archived` disposition. Direct durable and startup
  regressions prove no tool-call patch, tool-result append, event, Provider
  call, or unrelated job mutation; the parent bytes remain exact and only the
  existing bounded dead-letter audit state changes. Durable-record settlement
  time still owns grant validation. This focused repair does not close the
  wider 5.6/5.7 RC acceptance matrix, so both rows remain unchecked.

  The same-day Slice 17 focused candidate makes durable background-completion
  admission precede every live, manual/recovery, and startup lifecycle event.
  Each path reloads the latest child, rebuilds status/message and delivery
  identity from that record, reuses the existing one-lock exact completion and
  delivery-ledger owner, and publishes only after the exact completion item is
  delivered. Missing/archived parents, stale or malformed parent-tool
  identity, forged caller status/message, and exact-item conflicts therefore
  dead-letter or stop with zero background-completion lifecycle event.

  The completion item also acts as the existing durable outbox source for the
  admission-to-event crash gap. Under the existing durable event-store mutex,
  the canonical completion identity is reconciled against strict replay and a
  missing lifecycle batch is appended through the existing atomic event-bundle
  path. An exact published replay performs no append or live broadcast; a
  duplicate or mismatched event identity fails closed. Real
  `SubscribeEvents` and `LoadEventsSince` canaries block the exact completion
  ensure and prove zero lifecycle event before admission, then exactly one
  canonical durable-status event after release. Focused restart and 16-way
  live-plus-startup-recovery race checks retain one completion item, one
  retry/pending/delivered ledger item, one lifecycle event, and unchanged
  parent general-terminal CAS/publication bytes. The focused server, race,
  app/subagent, evidence, and architecture checks are source evidence only;
  package, Electron, Provider, formal, and wider RC acceptance remain unrun,
  so 5.6 and 5.7 remain unchecked.

  A same-day Slice 17 P1 re-review found that the initial focused candidate did
  not yet close the production restart cut: `RecoverPendingDeliveries` repaired
  a settled ledger and continued without reconciling its missing lifecycle
  bundle, while the live subagent wrapper reloaded durable state only when the
  caller record already appeared terminal. The append follow-up now reconciles
  only complete durable `delivered` completion identities through the formal
  `CleanupStaleRunningRecords` -> `RecoverInterrupted` ->
  `RecoverPendingDeliveries` startup sequence; `skipped` and `dead_letter`
  remain ledger-only. Every background subagent callback reloads the child and
  exact parent tool identity before classifying status, so forged terminal
  status/message or item/call/tool identity emits zero completion event.

  The same follow-up treats the complete canonical lifecycle bundle, rather
  than its single completion member, as the durable publication identity. All
  siblings must be byte-exact, ordered, and contiguous; incomplete,
  conflicting, duplicated, or diagnostically corrupt replay fails closed
  without append or broadcast. Formal restart canaries reopen both stores after
  admission with zero events, publish exactly one bundle on startup, and prove
  a second startup preserves identical event bytes and sequences. Live
  publisher errors produce only a bounded fixed stderr diagnostic, retain the
  delivered job for startup repair, and exclude hostile caller/error bodies,
  PII, paths, and authority references. These remain focused source checks and
  do not close 5.6 or 5.7.

  The 2026-08-25 Slice 24 focused candidate closes the production positive
  background auto-continue and exact recovery subdenominators without adding a
  second runtime, store, CAS, grant, or public protocol. An ordinary completed
  child may reserve one continuation after exact durable completion delivery;
  a case-bound child must instead rehydrate the existing host-trusted
  `ChildCompletionReceipt` with `CanContinueParent`. The existing jobs-manager
  lock linearizes the closed `starting -> started|skipped|failed` state across
  siblings, live callbacks, and startup recovery. The continuation uses one
  fixed value-free job/parent-bound carrier and the normal runtime-control turn
  path, so admission, Provider execution, and terminal publication still use
  the existing freeze and single finalizer. Reserved preflight precedes
  compaction and is repeated at the frozen boundary; exact reserved turns skip
  automatic compaction.

  Recovery distinguishes a durable reservation with no turn from an exact
  already-admitted turn whose status writeback was lost. The former revalidates
  current authority before starting once; the latter settles `started` without
  another Provider call. Completion-ledger and exact lifecycle reconciliation
  precede any started-notice repair. Only a successful `started` continuation
  appends the safe typed old-parent lifecycle; skipped, failed, hostile,
  stale-parent, sibling-conflict, and invalid-receipt outcomes remain durable
  job state with zero parent event. Notice authority is bound to the immutable
  `CompletionDeliveryAt`, while display time remains the later started time.
  Focused ordinary/case, crash-cut, concurrency/race, hostile carrier,
  authenticated HTTP/SSE, architecture, finalizer, and runtime composition
  checks cover this subset. Package/Electron, live Provider, formal, and the
  wider RC acceptance matrix remain outside this slice, so 5.6 and 5.7 remain
  unchecked.

<a id="snapshot-d469-case-tasks-648"></a>

### case-tasks: original lines 648–679

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `ee4c174653cf866eb899f9735d1a157936ac67f22246a42278d30fcef4f0a31e`.

  The 2026-08-23 Slice 12 verified subset removes `CaseOutcome` and every
  settlement-authority field from the generic serialized
  `case_source_status` projection instead of widening the TypeScript public
  schema. Go now emits the existing exact metadata-only
  `case_source_private|case_source_failed` shape, while the original strict
  host-normalized `ToolOutcome` remains process-local settlement input and is
  revalidated against tool/call, context digest/epoch, grant, case, immutable
  snapshot, server, transport/semantic/error/safety/receipt state, plus the
  existing opaque evidence-settlement path. The generic provider-safe
  projection also drops `toolOutcome` entirely. A shared Go-generated fixture
  is accepted unchanged by the strict runtime contract and renderer-facing
  public sanitizer; focused ordinary/race, durable history/fork, HTTP/SSE,
  provider-attempt, hostile zero-byte, and TypeScript checks passed. No
  Electron, package, live Provider, real user data, typed-local Funds
  lifecycle, or full RC matrix was run, so 5.9, 5.11, 5.12, 7.1b, and every
  wider formal row remain unchanged.

  The Slice 12 P1 follow-up adds one strict host-only binding proof to the
  canonical private durable case-source result after the complete process-local
  `ToolOutcome` settlement checks succeed. The proof binds version/purpose,
  outer tool/call/context digest/positive bounded epoch/grant, closed projection
  kind/status/code/message, and `isError`; public item/event/history/fork/SSE,
  Provider history, and the shared TypeScript contract never serialize the
  proof or outer authority. Read-time item, snapshot, event, Provider-history,
  and fork tests now recompute the proof and fail missing, malformed, duplicate,
  rebound, open, or unknown bindings to legacy withholding or safe omission.
  A real server case context, registered grant, settlement, durable append, HTTP
  detail, and SSE replay proves that the existing `case_boundary_only_v1`
  history policy safely omits the case turn without exposing or manufacturing
  ordinary reachability. Focused Go, race, cross-language fixture, hostile
  zero-byte, TypeScript, and strict OpenSpec checks passed; no checkbox or wider
  RC row is closed by this subset.

<a id="snapshot-d469-case-tasks-683"></a>

### case-tasks: original lines 683–773

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `ee777510309f08cbb971e333a26fc9cbafb994bd23f6602a02d6d08d53a6258e`.

  The 2026-08-25 Slice 25 candidate closes only the first current-code
  longitudinal retrieval gap. The existing caseentity owner now assembles one
  value-free selection with one total budget of 32 and the accepted seven-level
  priority, exact currentness, evidence/counterevidence binding digests,
  snapshot-pair binding digests, and fixed typed omitted coverage. Every
  provider reference is now a `CaseBindingHash`-scoped opaque digest rather
  than a raw private claim/evidence/gap/question owner ID; the same private
  scope also binds selection, snapshot, evidence, and counterevidence digests.
  Existing DSV2 manifests already content-bind tenant/user/workspace/case and
  `CaseBindingHash`, while focused canaries additionally reject transplanting
  case A's immutable snapshot into case B and prove same-case restart/replay/
  fork/resume stability without sending the private binding hash. The current
  finalized `ClaimRecord` owner appends an explicit versioned typed-state child
  for claim type; legacy schemaVersion=1 claim bytes keep a fixed golden digest,
  round-trip without the child, and cannot be implicitly backfilled during
  evolution. Loop ingress serializes only the unified selection, while the
  independent privacy projection re-parses canonical JSON, rejects unknown or
  hostile fields/references/order/coverage, and withholds AuthorityEntityRefs,
  source-exact values, PII, paths, raw provider/tool bodies, and reverse maps.
  Focused persistent-CAS and production HTTP evidence covers deterministic
  boundary/replay, historical-versus-current evolution, restart, resume,
  authorized fork, manual compaction, cross-case isolation, authority loss with
  ordinary additive continuity, and zero forbidden-field reflection through
  provider, HTTP, SSE, fork/resume responses, durable thread/event files, or
  generic public projection. The owner still has no complete longitudinal
  slot-to-retained-snapshot display binding that can safely reuse
  `AcceptedSlotDisplay`, so no second binding/resolver was invented.
  Search/export consumer filters, original retained-snapshot display
  re-resolution and `source unavailable`,
  explicitly joint-case revoke behavior, 5.10k automatic mixed compaction,
  5.11 TypeScript/Main/renderer work, legacy raw-tool migration, and the full
  matrix remain open; 5.9 therefore remains unchecked.

  The 2026-08-25 Slice 26 candidate closes only the private historical-display
  binding subset left by Slice 25. A successful production `PersistBoundary`
  now exposes one callback-scoped accepted-slot capability only after the
  complete trusted projection/event delivery succeeds; the existing
  caseentity longitudinal CAS then persists a value-free typed binding for the
  exact case, original thread/turn, committed accepted-final/disposition and
  Final Gate, immutable DSV2/context epoch, entity/slot, ClaimRecord and
  EvidenceReceipt digests, and currentness. Legacy V1 bytes/digests and absent
  child state remain unchanged, while unknown child/final-gate versions,
  partial/duplicate/rebound/tampered state, and non-canonical owner references
  fail closed. Restart, resume, authorized fork, and manual compaction inherit
  the exact binding through the current case owner. One Go-internal,
  host-resolved use seam accepts only the active TSCV2 plus the inherited opaque
  binding digest, revalidates the current case/principal/observation, committed
  V5 private final/disposition/claims/receipts/slot/entity binding, and reuses
  the existing retained DSV2 `AcceptedSlotDisplay` chain. It returns the
  original snapshot value or fixed `source unavailable` with zero exact bytes
  after deletion, corruption, revocation, or late authority drift, and it is
  not routed through HTTP or any public/provider/durable ordinary schema.
  Search/export filtering, explicitly joint-case revoke behavior, 5.10k mixed
  automatic compaction, the 5.11 TypeScript/Main/preload/renderer protocol and
  lifecycle, legacy raw-tool migration, packaged evidence, and the full RC
  matrix remain open; 5.9 therefore remains unchecked.

  The 2026-08-25 Slice 29 candidate closes only the current production
  search/export consumer subset. The real authenticated `/v1/threads` HTTP
  route now has a production-seam canary proving that canonical case-sensitive
  threads can match only the runtime-owned closed lifecycle status: hostile
  title, workspace, user/tool/result, accepted-final, source-exact, complete
  PII, path, relationship, and AuthorityRef bytes cannot act as a membership
  oracle, while ordinary title and user-text search remains additive. The
  unwired legacy `eventlog.Store.Search` has no production caller and therefore
  receives no new protocol or duplicate ownership.

  The standalone authenticated loopback thread reader now treats any response
  tree carrying an accepted-final strong marker as high risk, withholds title,
  workspace hash, and all user/assistant prose, retains only closed thread/tool
  lifecycle metadata, and never treats the bearer token or response-provided
  authority as an independent assistant-final authorization. Generic Desktop Write parses
  structured content through the existing recursive private accepted-final
  detector before workspace resolution, clipboard, save dialog, file, network,
  or preview effects; all four strong markers fail closed, while generic
  `envelope`, `registryHead`, `publicationIntent`, and `storeDigest` metadata
  remain legal. Hostile tool detail, citation, data URL, path, PII, private
  diagnostic, and raw-body bytes are either closed to metadata in the script or
  cause zero Write effects.

  The pre-fix canary run failed exactly `4/22` tests (`18/22` passed) in
  `1.87s`, reproducing case-user export and pre-effect Write admission. The
  final focused detector/script/Write/Main-IPC run passed `122/122` tests in
  `2.65s`; six exact Go app/thread, HTTP-adapter, canonical-store, and production
  handler search tests passed without test-result cache reuse, and focused Go
  vet plus root/runtime TypeScript typechecks passed. No package, Electron
  executable, live Provider, formal RC, full source-freeze, legacy 5.8b
  migration, or joint-case restart/revoke evidence was run or reclassified.
  Broader 5.9 package/formal and remaining fork/resume/history/compaction/display
  acceptance therefore remain open; 5.9 stays unchecked, while 5.8b and 6.12a
  remain POST_RC.

<a id="snapshot-d469-case-tasks-788"></a>

### case-tasks: original lines 788–807

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `09e1f58dd8efbef45384c1f71d6a4c6f5019cd9b0a8e93e19af44c159f924e3b`.

  The 2026-08-25 Slice 27 candidate closes only the authorized production-Go
  source/runtime composition subset. A real HTTP turn-start now drives the one
  existing `AutoCompactBeforeTurnV1` through a signed mixed case archive,
  retains independently terminal general authority without admitting forged
  ordinary turns, and restarts from the exact committed marker and ancestry.
  The source-available case request consumes only the original sealed typed
  continuation from that trusted marker together with the current bounded
  longitudinal selection; compacted general prose, raw case transcript,
  complete identifiers, private authority references, and reverse mappings
  remain absent. The same admission's exact hard-limit guard proves one
  automatic compaction, zero Provider transport, and one failed terminal, while
  a later legal-window request proves no duplicate marker or lifecycle pair.
  The existing caseentity currentness owner preserves the exact typed display
  binding across the automatic epoch transition, and the existing retained
  DSV2/local-display composition continues to return the original immutable
  value or fixed `source unavailable` after deletion or corruption without
  current/canonical/prose fallback. No TypeScript/Main/preload/renderer or HTTP
  public schema changed. Real packaged case continuation, Electron/public
  display lifecycle, current-package Provider evidence, and the remaining full
  RC matrix are still required, so 5.10k remains unchecked.

<a id="snapshot-d469-case-tasks-810"></a>

### case-tasks: original lines 810–829

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `fe92a2f4fa4621138e55037019bae9702c193d4e5ca7fed5b7752518b23ae72d`.

  The 2026-08-25 Slice 28 candidate closes only the authorized typed-local
  source/public lifecycle subset. The existing accepted projection receipt is
  still the sole renderer request source, and its thread/turn/public committed
  digest remains part of the renderer lease generation. The strict
  `typed-local-data-surface/v1` accepted response now requires that same
  lowercase SHA-256 digest; production Go derives it from the already
  validated committed V5 accepted final, and Electron Main rejects a missing,
  malformed, extra-property, or same-thread/turn mismatched response before any
  source-exact value can return to the renderer. The existing current
  main-frame/authenticated-principal checks, Go current-case/currentness and
  retained-DSV2 resolution, component-local full/masked lease, replacement,
  case/snapshot/session/principal/window/page/visibility/expiry/unmount late
  drops, and fixed browser unavailability remain fail closed. Accepted slot UI
  now renders only the localized field label and value, not the internal slot,
  claim/receipt, case/snapshot/digest, authority reference, or path. Focused
  contract, preload, Main, HTTP, renderer, store, browser, restart/reopen/fork/
  compaction, source-deletion/corruption, and generic-public privacy evidence
  is fresh; no package/Electron executable, Provider-live, formal RC, search/
  export, or explicitly joint-case evidence was run or claimed. Rows 5.9,
  5.10k, 5.11, 6.10, and 6.10b therefore remain unchecked.

<a id="snapshot-d469-case-tasks-854"></a>

### case-tasks: original lines 854–874

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `8e142765995756796a06713d2091d7412f5203d7f84c93167d638a89e4d82e6e`.

  The 2026-08-26 Slice 30R candidate removes exact institution from the V3
  provider-safe account-flow counterparty and advertised `host_funds` output
  schema when no bounded task-necessity plus combined quasi-identifier decision
  exists. An isolated baseline-HEAD archive reproduced both leaks before the
  candidate. Focused and complete domain/app/MCP/server tests prove the real
  producer, strict canonical validator, in-process MCP transport and V3
  `structuredContent`, exact advertised schema, settlement, public and
  subagent seams remain green; hostile institution/merchant/branch/geo/free-
  text/OCR/PII/path/SQL fields fail closed. Exact money, currency/scale,
  direction, balance, count, coverage, missing/null/invalid, query bounds,
  currentness, lineage and necessary identity semantics remain unchanged.
  Independently, the host-private resolver still receives the exact institution
  and preserves it in the validated protected-local `DisplayLabelV1`; neither
  that continuity nor this projection change promotes a Receipt, ClaimRecord,
  typed slot, local display completion, fact answer, or Final Gate. `gofmt`,
  touched-package vet, `git diff --check`, fresh OpenSpec status/instructions,
  and strict validation passed. Race was not added because no carrier, lease,
  concurrent-consumer, or ownership semantics changed. This is focused
  candidate evidence only: 6.3, 6.4, 6.8, and 6.9c remain unchecked, and any
  conditional institution-admission owner/contract or other quasi-identifier
  policy remains a separate Slice.

<a id="snapshot-d469-case-tasks-880"></a>

### case-tasks: original lines 880–924

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `120c65d3d604ef75a0a505e42bc507ee0c99ee2f85688f44bd05feafdc84bd87`.

  The 2026-08-22 Slice 7 candidate removes the production-reachable renderer
  flow factual cache and shared inflight dedup instead of inventing missing
  tenant/user/thread/binding/epoch/snapshot/source/schema/parser/query/
  argument/scope authority in the renderer. Legacy stats fact APIs remain
  fixed `source_unavailable`; caseentity alias resolution remains no-cache and
  store-backed. The four typed-local components share a 15-second local lease
  and clear on expiry, hidden/page replacement, unmount, authority drift, and
  late generation. Case-thread search is lifecycle-metadata-only, and Main
  log projection excludes refs/aliases, paths, raw SQL, and raw tool bodies.
  Recursive `eventlog.Store.Search` remains unwired/legacy; the production
  case-thread route searches only closed lifecycle status. Retained immutable
  DSV2/source snapshots have no accepted delete owner or TTL, and a missing or
  invalid bound source becomes `source unavailable`; flow-result cleanup is
  not counted as source deletion or retention.
  The Slice 7 P1 correction additionally removes the production-visible
  `AnalysisPage` warm-flow window cache/shared inflight and generic HTTP GET
  inflight coalescer; both are request-local, with current runtime authority
  checks and old-case abort/late-completion rejection. Renderer `log:error`
  now maps compatibility input to fixed Main-owned category/event codes plus
  allowlisted boolean/count metadata.
  The P1-C/P1-D correction additionally binds AcceptedSlotDisplay's validated
  thread/turn/publication-commit receipt identity into its request generation,
  so a prop replacement synchronously withholds the old response and
  invalidates old/late tickets before effect cleanup. Main managed-file and
  public-console logging no longer accept caller free-text fields at all: they
  emit only fixed Main-owned category/event codes plus exact allowlisted
  numeric/boolean/SHA-256/schema fields. The raw child log-line writer is
  removed and `analytix-process` can submit only normalized typed lifecycle or
  byte-count/SHA-256 capture records. Neutral exact text, file URIs,
  POSIX/drive/UNC paths, SQL/tool forms, upper/lower aliases, refs, and PII
  therefore have zero bytes in both direct logger sinks without relying on
  path or content regular expressions.
  P1-E makes the same closed projector total over hostile `unknown` detail:
  property-enumeration or getter failures produce no detail and only the fixed
  fallback event, so managed-file and public-console logging never throw.
  P1-F closes the remaining audited renderer console bypasses: the app error
  boundary retains only boolean state and sends no Error/stack/component
  detail; browser preview emits one fixed renderer event; rollback success
  emits nothing; embedded WebGL/wheel and runtime-id fallbacks emit fixed
  events without caller payloads. Existing GraphPage/FlowPerfPanel snapshots
  remain behind the numeric/enum projector, while embedded app/ops/host and
  Toast diagnostics remain fixed or allowlisted.
  This row remains open pending the wider 6.10b/6.10c lifecycle and RC/package
  public-seam matrix; the candidate does not use a focused source test as that
  final product claim.

<a id="snapshot-d469-case-tasks-928"></a>

### case-tasks: original lines 928–1321

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `6596e9ec3c174d7e2065c5778fb44db1cc186e36f5e66ab2d3a17e3ecad23aee`.

  On 2026-08-20 the coordinating Agent re-reviewed the production composition
  and reran focused Go tests across `runtimeapp`, `mcp`, `localdisplay`,
  `evidence`, `server`, `httpapi`, `nativecomponent`, `caseentity`, and
  `fundsquerysource`, plus the renderer/preload/Main/B1 contract slice. This
  originally reported closure of implementation-owned 6.1, 6.2, 6.10, 6.10a,
  and 9.8. The 2026-08-21 code-level review supersedes that conclusion for
  6.10: the current AcceptedSlotDisplay resolves the private binding canonical
  value rather than the retained original snapshot source file/row/field value.
  Therefore 6.10 and 6.10b remain open; 6.10b specifies the retained
  source-exact lifecycle. The same run's real Electron diagnostic failed before
  its first ordinary turn, so 6.12, 7.9, and every Electron/package/formal row
  remain open.

  The 2026-08-21 Slice 3 correction repaired the narrow retained
  `AcceptedSlotDisplay` source-exact defect without changing publication
  authority or adding another runtime, database, registry, grant, CAS, or
  protocol. Production account-flow settlement now derives a closed
  `account|card` source field from the already verified `acct:<n>|card:<n>`
  subject alias and stores a private, value-free V3 canonical-evidence binding
  from the exact inflow/outflow/count fact set and AuthorityEntityRef to the
  witnessed opaque source-file identity, `SourceRowRecordV1`, row number, and
  declared field. The retained resolver reads only the declared same-type
  `norm|raw` pair, verifies the original manifest/ledger/lineage/parsed page,
  and rejects account-card fallback, missing/conflicting fields, locator drift,
  and canonical mismatch. `full` and host `masked` consume only the original
  raw value returned by that single callback; canonical/current/typed-label
  values are distinct sentinels and are never display inputs.

  OBSERVED synthetic production-composed evidence now crosses one real V3
  receipt with two-step raw-to-native-to-canonical lineage, three verified
  inflow/outflow/count claims, Final Gate, committed durable V5 private final
  and disposition, a durable private caseentity binding closed and reopened,
  the real `fundsquerysource.Service.UseRetainedAcceptedSlotDisplay`,
  `localdisplay.Service.AcceptedSlotDisplay`, and `LocalDisplayHandlerV1`.
  Both account and card cases return the old naturally formatted retained DSV2
  value with `Cache-Control: no-store`; masked mode retains identical claim and
  receipt IDs. Changing the current snapshot does not upgrade the historical
  value. Retained deletion/expiry, corrupt material, receipt revocation, late
  principal drift, and an independent thread attempting to open another
  thread's old final all return the fixed conflict/unavailable projection with
  zero exact bytes. Separate focused resolver/service tests cover wrong
  file/row/record/field/snapshot/manifest/epoch/case/claim/receipt, cross-field
  conflicts, missing lineage, same-material drift, and callback reuse.

  OBSERVED durable server and private-owner tests separately prove restart,
  reopen, fork, resume, compaction, and explicit same-case independent-thread
  rehydration of typed aliases/currentness/evidence references without prose or
  generic Memory. The independent-thread contract starts new same-case work; it
  does not directly open another thread's historical final. Main/renderer
  focused tests separately prove the closed public TS contract, Main/preload
  routing, component-local response state, and zero-byte invalidation on
  case/snapshot/renderer-session/principal/main-window changes and late replies.
  These are compositional lifecycle observations, not a claim that each
  topology was driven through one real Electron process.

  The 2026-08-23 Slice 13 candidate restores the deterministic accepted-turn
  reachability fixture to the production `qscope1_<sha256>` query-scope shape;
  its prior fixed HTTP 409 was a stale fixture that produced no Final Gate
  account-flow outcome or eligible slot, not a relaxed authority check. The
  corrected account/card chain now asserts one eligible slot before sealing the
  real committed V5 private final, then resolves both full and masked values
  from the retained original DSV2 source file/row/field. The Go HTTP adapter
  reloads and exactly compares the current TSC immediately before returning, so
  a late case switch, snapshot evolution, or epoch advance discards the already
  resolved value. Electron Main similarly pins the closed Hub authentication
  source, principal, and session snapshot across the request; logout, principal
  replacement, session refresh, main-frame/window drift, and renderer lease
  revocation keep late bytes out of the returned typed result. The actual
  host-private account-flow carrier passed a 64-consumer race with exactly one
  summary/row callback, while restart reconstruction, duplicate consume, and
  replay remained closed. Focused Go, Go race, typed contract, Main, preload,
  renderer, browser-bridge, target ESLint, diff, and strict OpenSpec checks are
  the scope of this candidate. No formal package/Electron discovery, Provider,
  live-data, upstream, or full-matrix evidence is claimed, and no open task row
  is closed by this note.

  A Slice 13 P1 correction now denies accepted-slot requests before the Go
  local-display boundary unless Main observes an authenticated host-owned Hub
  or permitted test-bootstrap source, a non-empty current principal, and a
  non-empty stable session `checkedAt`. Logged-out defaults and malformed
  authentication/source/principal/session combinations return the fixed closed
  `403 forbidden` result without issuing a local-display request or exposing
  source-exact bytes. Positive and late-window fixtures now supply an explicit
  valid Hub snapshot; the existing return-time logout, principal-switch, and
  session-refresh comparisons remain intact. This note closes no task row.

  A production CanonicalEvidence consumer audit found complete V3 bodies only
  in the existing private settlement/registry/CAS owners. Claim, Final Gate,
  report-publication, and registry-publication projection consume facts only;
  controlled-PII authorization requires V2; typed local display is the sole V3
  binding consumer. Provider output, MCP result, ordinary history, final text,
  generic HTTP/SSE/event/renderer contracts, and typed local response assertions
  contain zero locator or AuthorityEntityRef bytes. The audit also found and
  closed the generic restricted-evidence sanitizer's missing V3 recognition.
  A follow-up P1 review then reproduced that an otherwise valid V3 JSON object
  followed by ordinary prose escaped because the old candidate extended to the
  end of the string, and that the first 32 candidates plus standalone
  `cer1_...`/`srow1_...` references could bypass generic string/value/SSE/log
  projection. The bounded scanner now extracts complete balanced object/array
  slices with string/escape/nesting handling and fails closed on byte, depth,
  node, or candidate-budget exhaustion; the same existing closed internal-ref
  detector now gates serialized text, runtime values, SSE/event admission, and
  managed logs. OBSERVED fresh focused evidence passed 269/269 core public
  projection/SSE/Main/renderer/logger tests, 126/126 direct sanitizer-consumer
  tests, the Electron HTTP projection target, root TypeScript typecheck,
  runtime-package typecheck, and 22/22 runtime conformance tests. Benign prose/
  ordinary JSON, `acct:1|card:1`, and an ordinary 64-hex value remain admitted
  by this restricted/ref boundary.

  A second follow-up P1 review reproduced that public assistant text still
  admitted embedded V3/locator content and closed `cer1_...`/`srow1_...`
  references, while the ordinary public-text projector still admitted those
  closed refs. Assistant text now reuses both restricted/ref detectors, and
  ordinary public text reuses the ref detector, including for ASCII and JSON
  Unicode-escaped spellings, without changing short aliases, ordinary hashes,
  or benign prose. OBSERVED production-callpoint regressions cover renderer
  `splitThink`, short/edit inline-completion actions, Claw failed-turn errors,
  the already-serialized completed-reply path, and the non-JSON runtime HTTP
  response catch. A complete direct-consumer audit confirms original user
  input still reaches private host turn/steering/user-input submission paths
  exactly while only optimistic/display/history/export/external-mirror state
  receives the public projection. Fresh focused evidence passed 225/225 tests
  across nine shared/Main/renderer files, three targeted Electron HTTP/body
  projection cases, and root TypeScript typecheck.

  A final 2026-08-22 P1 review reproduced that purpose-only CanonicalEvidence
  V3 and source-row-locator V1 partial envelopes without any `cer1_...` or
  `srow1_...` reference still passed the ordinary public-text projector and
  could reach Connect Phone bridges. The projector now reuses the existing
  acyclic restricted-evidence detector as well as the closed ref detector, and
  Connect Phone rejects an empty public projection before selecting or calling
  WeChat, Telegram, or Feishu. OBSERVED fresh evidence passed 244/244 tests
  across ten shared/Main/renderer consumer files plus root TypeScript
  typecheck; the private host still receives original user turn, steering, and
  user-input submission values while renderer/history/export/IM projections
  remain fail closed.

  The 2026-08-22 Slice 4 candidate establishes the framework-only
  `typed-local-data-surface/v1` closure without claiming Import/Cleaning
  product reachability. OBSERVED the public runtime TypeScript contract and
  Go domain contract now compare as one exact four-kind family with closed
  request/response fields, status enums, opaque selectors, stable requested
  cell order, `0..100000` offset, `1..100` limit, 4096-byte logical cells, and
  a 1 MiB response ceiling. DirectSourcePreview and AcceptedSlotDisplay retain
  their existing use cases; their requests now participate in the same closed
  discriminated family, and retained-source regressions remain green.

  OBSERVED synthetic/conditional Go authority readers bind Import Mapping to
  the current staged/import, source-item, parser, and mapping generations, and
  Cleaning Diff to the input snapshot, rule generation/digest, output
  snapshot, and transform lineage. Both revalidate the current host principal
  and the surface authority after the exact-value callback; wrong selector,
  generation, input/rule/output/lineage, row window, reordered/missing cells,
  callback reuse, oversized value/response, and late drift fail closed. Full
  and masked synthetic projections preserve identical selectors and lineage
  while changing only host-returned display bytes. Their dependency graph has
  only identity plus the surface-specific immutable reader, with no Agent,
  Provider, MCP, EvidenceReceipt, ClaimRecord, or Final Gate dependency.

  OBSERVED fixed Go HTTP routes, Main IPC channels, preload methods, explicit
  browser-unavailable shims, and unmounted renderer loader/components exist for
  Import Mapping and Cleaning Diff. Main sends only the closed renderer request
  to those routes, rechecks the current main frame and Main-owned workspace /
  runtime generation before and after the request, rejects selector/field/page/
  mode or response-schema drift, and keeps all local-display paths unreachable
  through generic `runtime:request`. Renderer exact bytes remain in
  component-local state, are synchronously withheld after authority/selector/
  page/mode changes, and no new copy/export/share/print/Connector/email/upload
  surface was added.

  CONDITIONAL production composition intentionally supplies no Import Mapping
  or Cleaning Diff reader, so both routes return source unavailable. No real
  acquisition/parser/mapping producer, deterministic cleaning producer,
  legacy Import/Cleaning UI wiring, Electron run, package, or real user data
  was used. Therefore code/source presence is not product reachability and
  6.10c, 7.1c, 7.9, and all wider rows remain unchecked.

  OBSERVED a 2026-08-22 Slice 4 privacy follow-up closes a generic-projection
  gap without changing the numeric `schemaVersion: 1` wire contract. The
  shared TypeScript and Go restricted-evidence detectors now treat only the
  exact pair `schemaVersion == 1` plus one of the four closed typed-local
  `kind` values as a private response identity, including below neutral
  wrappers and in bounded serialized JSON. Four PII-neutral synthetic
  canaries are withheld by TypeScript value/text/SSE projection, managed logs,
  and ordinary thread-detail history, and by Go value/canonical-text
  validation plus public event/durable-history projection. Prose mentioning
  only the family or kind name, incomplete identities, wrong versions, and
  unknown kinds remain public. Focused dedicated Main/preload/browser/
  renderer and Go app/HTTP/domain tests remain green, so the typed local sink
  still receives its closed response without passing through the generic
  projector. The same follow-up also makes the DirectSourcePreview app seam
  independently reject `rowIndex` above 100000 even if a future runner returns
  sequential rows that bypass the current native parser's earlier check.

  Rows 6.10, 6.10b, and 6.10c remain open: no real Electron/package run
  established one end-to-end lifecycle across every restart/fork/compaction
  topology, and their wider Definition of Done remains incomplete. Rows 5.11,
  5.12, 6.3, 6.4,
  6.8, 6.11, and 6.12 likewise remain open; no broader row is closed by this
  retained-source correction.

  The 2026-08-22 Slice 5 candidate replaces direct Main-selected CSV admission
  with a dedicated host-local `stage -> ImportMappingPreview -> explicit
  confirm|cancel` path. The Go owner freezes one current-case CSV/ZIP, parses
  UTF-8-or-GB18030 CSV members without filesystem extraction, creates a
  complete stable private inventory and short per-item selectors, and keeps
  one bounded ten-minute process-local generation. Confirm reopens and
  revalidates the same source, case, principal, inventory, parser/mapping
  policy and deterministic canonical 34-column CSV before reusing the existing
  immutable-source/material/native/DSV2 admission. Cancel, reselect, authority
  drift, expiry, shutdown, and failed/canceled confirmation clear exact staging
  bytes; unconfirmed state is intentionally unavailable after restart.

  Stage/reselect ordering is defined at invocation start in both Main and Go.
  A later invocation or shutdown cancels the prior in-flight stage, and a late
  old response can only issue a selector-matched cancel for its own selector;
  it cannot install, clear, or cancel the current generation. Confirmation
  failures proven before the existing DSV2 CAS leave the old head unchanged.
  Response loss after that CAS is outcome-unknown and must be reconciled from
  the existing current DSV2 authority; it is not claimed as rollback.

  The renderer request/response surface contains only selector, closed fields,
  page and display mode plus fixed path-free inventory metadata. The existing
  Session Header data-import action now mounts ImportMappingPreview with full
  default, masked option, component-local exact bytes and late-response
  invalidation. Browser fallback remains unavailable. Production composition
  supplies the Import reader while the Cleaning Diff producer remains absent.
  The original selected CSV/ZIP and member inventory remain non-authoritative
  pre-snapshot memory; confirmation admits the deterministic canonical accepted
  CSV through the existing raw/parsed/source-row/DuckDB/DSV2 owners.

  Supported bounds are exactly ordinary CSV or regular CSV members in a
  non-encrypted store/deflate ZIP; 64 MiB source, 16 members, 48 MiB per member,
  64 MiB aggregate expansion, 200:1 expansion after a 1 MiB allowance, 100,000
  aggregate rows, 64 columns, 16 KiB header, 4,096-byte source cells, 1 MiB
  typed response, and ten-minute staging TTL. XLS/XLSX/7z/RAR/password ZIP,
  nested archives, ZIP64, external extractors, durable unconfirmed recovery,
  and cleaning are not implemented or claimed. Windows source acquisition is
  deliberately unavailable until its owner/hardlink identity has a tested
  platform-specific implementation.

  ZIP admission first performs a bounded EOCD/central/local-header structural
  preflight, including the 1..16 member cap, before `zip.NewReader`. It rejects
  multi-disk and ZIP64 structure, non-UTF-8 member identity, local/central
  disagreement, and every general-purpose flag outside data-descriptor, UTF-8,
  and valid Deflate option bits. Empty CSV members remain rejected by this
  Slice 5 member policy; zero compressed size is not asserted as a ZIP-format
  rule. ZIP64 magic in ordinary member data or an ordinary central-file comment
  is not treated as archive structure.

  This focused candidate is not formal Electron/package/B1 evidence. Cleaning
  Diff, the full four-surface lifecycle, wider generic-channel/package matrix,
  retained historical Accepted Slot source, and formal cross-platform runs are
  still incomplete. Therefore 6.10, 6.10b, 6.10c, 7.1c, 7.9, 9.11, 10.4, and
  all wider rows remain unchecked.

  The 2026-08-22 Slice 6 candidate follows the accepted Cleaning Diff contract
  as a post-operation preview. The active spec binds immutable input snapshot,
  closed rule generation/digest, immutable output snapshot, and transformation
  lineage, but defines no cleaning-specific staged confirmation. Accordingly,
  this slice adds no prepare/confirm/cancel protocol, staging registry, second
  current pointer, or mutable legacy cleaning path. One fixed-rule native
  operation reads the current DSV2 through the existing inherited-handle Funds
  source, emits a deterministic canonical CSV and ordered exact diff, and
  reuses the existing canonical Funds material builder plus the same sealed
  DSV2 witness CAS with an expected-current predecessor check.

  A proven pre-CAS failure preserves the prior head. Response loss or authority
  drift after possible CAS success returns `outcome_unknown` and reconciles
  only through current DSV2; it never claims rollback. A successful current
  output mints one bounded ten-minute component-local CleaningDiffPreview
  generation. Wrong/stale selector, input/rule/output/lineage, case/principal,
  page/field, expiry, rerun, renderer loss, restart, and shutdown fail closed.
  Electron Main/preload and the cleaning page carry only the opaque selector
  and closed lineage/count metadata; exact full/masked cells stay inside the
  existing typed-local component and browser fallback remains unavailable.

  Focused synthetic Rust, Go, Main/preload/renderer, contract, and production
  composition evidence does not constitute the wider source-free, Electron,
  package, retention, or formal B1 matrices. Therefore 6.10c, 7.1c, 7.9,
  9.5, 9.11, and every wider row remain unchecked.

  The Slice 6 P1 follow-up narrows the production rule to changes that the
  typed diff can express. Canonical-builder-admitted
  `counterpartyAccount=CP-001_ 23` now produces exact
  `CP-001_ 23 -> CP00123`, one changed row/cell, an output accepted by the same
  builder, and an unchanged deterministic replay. Changed rows with no
  allowlisted cell fail closed in both Rust and Go; non-display balance
  normalization is not hidden behind an empty diff. Native result rows/cells
  are incrementally bounded under the shared complete-wire 1 MiB/32,768-token
  limits; Go reserves the exact fixed 26-token native response envelope on top
  of that inner cap, and private carriers reject JSON and redact all fmt
  variants.

  A closed `pre_cas_failed` operation status reaches Main only when the host
  proves the CAS was not called or fresh current-head reconciliation still
  resolves the expected input; it displays as local `unavailable`. Unknown
  admission errors, a different/unavailable reconciled head, post-CAS response
  loss, malformed/non-success transport, and later authority drift remain
  `outcome_unknown`. The production Main typed-local allowlist now admits only
  the fixed cleaning run/revoke paths alongside the existing closed family and
  rejects path variants before transport. These focused P1 checks do not close
  any wider task.

  The 2026-08-21 Slice 2 candidate completed the narrow provider-safe exact
  account-flow seam without introducing a second runtime, database, authority,
  registry, or protocol. The current host projection now carries the closed
  `current` state, settlement binds the native `queryHash`/`resultHash` through
  a two-step raw-to-native-to-canonical lineage, and Final Gate isolates the
  untyped provider body only when all accepted claims partition by unique
  receipt into one or more complete inflow/outflow/count groups, each resolving
  one internally consistent current query scope, exact result lineage, TSCV2,
  DSV2, snapshot and epoch. Multiple complete query scopes retain separate
  host-rendered groups in a partial envelope without retaining or combining
  provider prose; any incomplete, rejected, partial, mixed-tool,
  cross-receipt-mixed, or legacy lineage group retains the pre-existing
  ordinary/fail-closed handling. Final rendering
  derives each exact net only from its settled integer claims; it does not parse
  or correct model prose. Production-seam tests cover wrong single- and
  cross-query model arithmetic, amount-free prose, long AuthorityEntityRef/PII
  drafts, missing/rejected/partial/mixed claims, failed terminals, legacy
  unbound result lineage, other tools, and ordinary chat. An independent
  synthetic DuckDB `DECIMAL`/`HUGEINT` oracle compares
  money, currency, scale, direction, count, bounds, coverage, null/invalid
  rejection, currentness, evidence ordering and query/result binding through
  `analyze_account_flows`, and a deliberate `DOUBLE` fixture diverges from the
  receipt-eligible exact result. Focused Go production composition/server,
  Rust account-flow, and plugin closure/schema checks passed with no Provider,
  real user data, Electron, package, or source-exact display run.

  This evidence is a verified sub-slice only. Rows 6.3, 6.4, 6.6, 6.7, 6.8,
  6.9c, 6.11, 6.12, and 6.12b remain open because their typed answer/source-
  field slots, complete local-surface positive matrix, protected-source bypass,
  provider-token, historical/currentness, package/Electron, or broader
  cross-capability requirements were not completed by this slice.

  The 2026-08-23 Slice 11 candidate adds only the provider-safe typed outcome
  and answer-slot eligibility implementation sub-slice. The account-flow host
  output atomically advances to a strict closed v3 shape whose independent
  aggregate, evidence-row, typed-slot, local-display, factual-answer, query-
  scope, currentness, lineage, and source-field axes cannot authorize one
  another; the existing host `ToolOutcomeV1` remains the separate transport
  success/failure owner. The value-free `afslot1_...` source-field reference
  hashes the exact native query hash, native result hash, and closed
  `account|card` field. The native query hash already seals case binding,
  context digest/epoch, immutable snapshot, private subject resolution, query
  bounds, and the fixed query contract; a focused case A/B regression keeps
  alias, bounds, amounts, currency, scale, count, and field identical while
  proving different query/result hashes and unlinkable references.

  Host transport and native validation independently recheck the closed
  outcome plus exact integer money, currency/scale, direction/count, coverage,
  pagination/currentness, and query/result hashes. Final Gate is the only
  eligibility upgrader: it requires one complete current V3 canonical evidence
  group per receipt, exact inflow/outflow/count fact bijection, same receipt and
  query scope, current case/binding/epoch/snapshot, exact two-step native-to-
  canonical lineage, and retained private accepted-slot source bindings.
  Eligible output still records local display as `not_requested`; display and
  Direct Source Preview create no receipt, claim, or factual authority. The
  existing private evidence carrier remains exactly-once and the provider and
  final typed shapes reject unknown raw PII, AuthorityEntityRef, reverse-map,
  path, SQL, locator, and source-value fields.

  Fresh focused Go package and public settlement tests passed without test-
  result cache reuse, as did focused account-flow race tests, the existing
  DuckDB/native exact-field oracle (`1/1`, including deliberate `DOUBLE`
  divergence), production MCP entry closure, and tool schema contracts
  (`62/62`). Public TypeScript contracts are `NOT_AFFECTED`: no TypeScript wire
  shape changed in this sub-slice. Electron, package, live Provider, formal
  B1/full matrix, real user data, foreground-child handoff, and longitudinal
  rehydration are `SKIPPED_BY_SCOPE`; Codex/DeepSeek comparison is likewise
  `SKIPPED_BY_SCOPE` because there is no concrete upstream failing seam. Rows
  6.3, 6.4, 6.8 and all broader dependent rows remain unchecked.

  The same-day Slice 11 P1 append closes two narrower structural acceptance
  gaps without changing the typed outcome wire shape or adding authority.
  Final-answer domain validation now requires each outcome group's valid
  `qscope1` reference to equal the sole source id in the identical
  `SupportedScope` shared by its exact inflow/outflow/count claims; both New
  and Parse reject a rehashed, valid-format cross-scope reference.
  `AcceptedSlotDisplay` now independently re-closes the unique group/receipt
  and exact slot claims against complete pagination, identical receipt/claim
  query scope, receipt query hash, exact two-step raw/native/canonical hash
  chain, recomputed `afslot1` reference, and the unique closed field in the V3
  retained source bindings before any source-exact callback. Shape-valid wrong
  qscope/query/result hash, recomputed forged reference, partial pagination,
  legacy one-step lineage, and wrong source field all fail with zero exact
  callback/response. Local display remains `eligible` and `not_requested`;
  the sealed-registry private-final preflight still reruns Final Gate and stays
  the factual authority owner. Focused ordinary and race checks across domain
  evidence, local display, Final Gate/evidence settlement, and the server
  public account-flow settlement passed. P2 claim-shape/tool-identity expansion
  and the wider RC/package/Provider matrices remain outside this append, so
  6.3, 6.4, 6.8 and all dependent rows stay unchecked.

<a id="snapshot-d469-case-tasks-1326"></a>

### case-tasks: original lines 1326–1329

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `c5aa3ba9c81d46adc3bf8ea2d80b089f2e974a91e9e8ae90618b1ee5979a826d`.

  Slice 7 adds only the executable disabled/plaintext-integrity posture and a
  synthetic record-byte guard. It deliberately implements no encryption,
  key store, envelope, migration, rotation, deletion promise, or crypto live
  test, so 6.12c remains open.

<a id="snapshot-d469-case-tasks-1338"></a>

### case-tasks: original lines 1338–1362

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `dc9766f8c6faefaba4f2dd6b71b443f775784adbf0ec68a66f4d377dbb89ca5d`.

  The 2026-08-23 Slice 18 focused candidate rebuilds supported renderer WS
  events as a closed `SafeWsEventEnvelope` before `actions.setWsEvent`, keeps
  only validated case/job identity after exact `cer1_`/`srow1_` private-reference
  rejection, event metadata, sequence/timestamp, fixed
  status/level and bounded progress/step/counters, and withholds unknown event
  or type shapes. The shared transport projects before `lastEvent`, generic
  listeners, `getLastEvent`, or `emitLast`; its two runtime-control event names
  remain private, closed control-owner inputs and never enter generic event
  state. Runtime status now binds a frozen pending action/session to the exact
  response and admits cache updates only for an enrolled, case-bound session
  under current authority; unknown, wrong-request, wrong-session, private-ref,
  numeric account/phone, cross-case, and revoked inputs commit nothing, while
  reconnect restores an enrolled safe status and authority revocation clears
  both safe caches. Request ids use the same exact private-ref/numeric
  exclusions; a duplicate pending id fails before timer/map/send creation, and
  timeout/send cleanup can remove only its exact pending entry. Synthetic
  runtime canaries cover complete account/phone,
  long AuthorityEntityRef, source-row refs, reverse-map-like objects,
  DuckDB/local paths, raw rows, nested diagnostics, throwing
  accessors/`toJSON`, and hostile Proxies; the resulting global runtime state
  contains zero hostile bytes while normal progress, cleaning completion
  refresh, and the sole closed `JOB_CANCELED` cancellation status remain
  present. The typed local source-exact sinks are unchanged. This is focused
  source/runtime evidence only: no Electron/package/Provider/formal/full-matrix lane ran, so
  6.10c and 7.1c remain open.

<a id="snapshot-d469-case-tasks-1385"></a>

### case-tasks: original lines 1385–1817

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `048e0a4ca03da05cdb21a21f03552d54db77381efe45cca7385805ccc82fa930`.

  2026-09-09 R129 W3 worktree checkpoint (base `cb3c9cbe5`): complete
  projected/rejected report histories and a controlled-full report with closed
  V2 access now pass native runtime startup/restart while retaining the original
  publication records. The historical proof uses actual persisted Core results,
  independently enrolled witness observations, source-bound artifact bytes and
  signed records. R129-511/512/513 passed the two-restart controlled case,
  incomplete-history and before-Apply refusal controls, signed Final removal of
  the entire closed inventory, and rejected-history compatibility. Complete
  grant/receipt/disposition bytes, modes and presence are compared independently
  of the open-only restart plan. These are synthetic native historical fixtures,
  not Funds query, Provider/UI, live approval or release evidence. R129-516/518
  additionally passed crash-open and mixed closed/open V2 recovery through the
  existing stage-only consumer, with full-length `release_indeterminate` and
  exact closed-record preservation. R129-520 established one applied signed
  disposition with another still open, then passed full runtime recovery and a
  fresh restart; the cut was injected at semantic Apply inside owner retirement.
  The high-level cancellation cut in 518 was not established, and 519 skipped
  on an unsuitable secure-config volume; neither is a passing interruption
  result. R129-533 additionally passed the exact `materials_durable` deferred
  cut: two native starts preserve original materials, primary, pending receipt
  and absent disposition; current-key re-signing cannot substitute a foreign
  independently enrolled installation/enrollment, alter the bound stage input,
  or erase the signed Final inventory. The report stays held without recovery
  effects. R129-536 passed the local synthetic-provider HTTP plan/tool, protected
  boundary, ordinary continuation, normal shutdown and fresh history recovery
  with the SharedEvidence witness unavailable on restart. Two new boundary
  finals perform their own registry observations before shutdown; report
  preservation and restart add none, and no witness advance/resolve occurs.
  The joint test exposed a live `ReplayAt` startup dependency (531); the signed
  sequence-zero historical prefix now receives exact context/head validation,
  with full evidence-package controls passing in 535. Nonzero historical heads
  retain their strict replay requirement. Other partial report cuts, nonzero
  historical replay isolation and full A–D integration remain open. R129-539
  additionally passed native two-start preservation for unresolved `reserved`
  and `candidate_durable` cuts. R129-546/548 admit the three pre-witness cuts
  when the actual pending disposition is Failed: the report is closed without
  holding its thread, and all four publication owners and the selected pending
  receipt/disposition retain their exact bytes, modes and presence. Failure
  before the caller persists its Core tool result is a legal producer crash
  cut; recovery adds the actual metadata-only failed result once. Original
  results remain exact, and an originally absent result may only receive the
  complete real Core failure projection. Failed/Cancelled producer and replay
  controls passed the full pendingwork/executiongrant packages in 547; native
  startup evidence currently covers Failed. R129-548 passed signed semantic
  mutation controls and UNKNOWN, foreign identity and Final-erasure refusals.
  Oversized Original results and wrong-role candidates fail at the earlier
  Core validator, not at a claimed reachable later comparison. R129-547 passed
  an actual local-provider ordinary read and final on the same formerly
  interrupted thread, then shutdown/restart with exact history and no provider
  replay. Its combined command was partial because two negative assertions
  expected the wrong refusal boundary; those expectations were corrected and
  passed in 548. Closed-history startup still performs SharedEvidence
  observations (10 on first start, 22 cumulative after the second), with no
  advance, resolve or checkpoint change; this is not offline isolation proof.
  R129-543 showed only pending-directory metadata changes, with no report or
  pending-record file drift; it did not establish directory removal. Other
  closed statuses, post-witness partial cuts, nonzero historical replay and
  settlement inventory isolation remain open. This checkpoint does not close
  9.8a or establish Product/Formal RC.

  R129-550/552 additionally prove qualified V2 nonzero boundary-final history
  through complete Original registry graphs and independently enrolled stored
  witness exchanges. Selection follows the witnessed index ancestry and binds
  the full frozen context and signed head; unobserved local capsules cannot
  select themselves. The qualification, including its absence, survives the
  same startup's stage/activation recursion and explicit Prepare/Activate.
  Before Apply, the candidate must retain immutable Original registry/witness
  materials and existing boundary finals, and its full private-final graph
  must validate under the current key. R129-552 passed two offline preflights,
  harmless metadata Apply, actual Apply erasure/foreign-final refusals, signed
  deletion refusal through configured startup and maintenance, and rejection
  of genuine witness exchanges supplied only by signed Final. Refusals make
  zero managed writes and witness calls. The configuration-free maintenance
  entry now refuses transactions changing these canonical history records
  without configured Original authority; unrelated maintenance passed in 551.
  Fact variants/admissions and current membership retain their existing paths;
  legacy or originally unavailable graphs acquire no new qualification.
  Full evidence-package checks and held Prepare/Activate/restart compatibility
  passed in 551. Its three prospective negatives initially asserted at Prepare
  rather than the actual Apply guard; that combined command remains partial,
  with corrected actual-Apply controls passing in 552. The closed legal
  failed-before-result two-start case passed with SharedEvidence Observe 7 on
  the first start and 13 cumulative, without advance/resolve/checkpoint change.
  R129-552 Windows evidence/runtimeapp checks are compile-only. Whole-startup
  witness-outage isolation, no-hold settlement inventory and remaining report
  cuts are still open; none of these results closes 9.8a or Product/Formal RC.

  R129-554/557 extend that qualified Original V2 slice with complete settlement
  history audit before the first writer: current-key prepared records, strict
  two-family Core primaries, historical grant/result/marker prefixes and every
  capsule issue must agree in both Original and projected Final. Prepared-only
  and marker-before-capsule are auditable history, never a repair capability.
  Frozen prepared bytes/modes and original marker results survive the same
  startup qualification; no held thread is invented. The no-hold observer uses
  this qualified inventory instead of live registry listing. R129-554 passed
  the evidence package. R129-555 retained an actual nil-journal rejection;
  corrected absence revalidation passed in 556. The legal closed-before-result
  two-start case then observed SharedEvidence once on the first start and once
  cumulatively, with no Risk calls or witness advance/resolve/checkpoint change.
  R129-557 passed a complete existing-nonzero-boundary startup/shutdown with
  SharedWitness offline and zero witness calls, plus actual Apply rejection of
  prepared/marker deletion, conflicting or incomplete primary directories,
  registry/witness erasure and foreign final addition; ordinary title Apply
  passed. An actual signed prepared-deletion program was refused by configured
  startup and configuration-free maintenance before writing. Runtime evidence
  in 557 took 129.647s. This proves the qualified existing-boundary restart,
  not initial creation of a new Core boundary during an outage, all no-hold
  states, other report cuts, current fact authority, or full A-D/9.8a acceptance.

  R129-559 reproduced the remaining first-boundary outage at runtime composition.
  The fresh remote Observe producer now identifies only pure witness
  unavailability; cancellation, mixed errors and local projection/floor failures
  cannot acquire that classification. The boundary consumer uses it only before
  the locked-snapshot callback begins, then persists the existing fixed non-fact
  boundary through current-key private/CAS/terminal authority. Its context-bound
  zero head belongs only to that new boundary; Original registry is untouched.
  Callback/post-publication failures never trigger a second mint. R129-560
  evidenceauthority tests passed; its evidence test build failed on a nonexistent
  test field and native execution was not reached. R129-561 evidence tests passed;
  native startup reached a witness-count assertion that conflated failed HTTP
  attempts with successful Observe calls. R129-562 passed the corrected exact
  accounting and native first-outage two-start recovery, with one failed Observe
  attempt total, zero successful observations and unchanged witness checkpoints.
  Its same-thread ordinary HTTP read, two local test-Provider calls, persistence,
  shutdown and restart also passed without Provider or witness retry (106.03s;
  combined runtimeapp 207.794s). R129-563 passed signed Cancelled materials-durable
  history both with an existing tool_cancelled result and before that result,
  two offline starts each (runtimeapp 204.642s). It retains the exact signed
  report_stage_cancelled reason and durable failed lifecycle without UNKNOWN
  conversion or changing Original records. This proves historical recovery of
  those Cancelled cuts, not live Publish cancellation production. Post-witness
  partial cuts and joint A-D/full 9.8a remain open; no Product/Formal RC claim.

  R129-565 reproduced the global reconciliation gate at the exact post-witness
  committed_settlement cut: the real CommitExact intent and committed receipt
  were persisted and read back before Observe/selection/commit/decision/Core
  result/disposition. The single-attempt cut now supplies preservation and
  denial only after complete Original/Final equivalence, current-key graph
  validation and independent enrollment-pinned Advance receipt verification.
  R129-566 passed two offline native starts (runtimeapp 139.786s), retaining
  the complete Original and every missing suffix without witness requests.
  R129-567 passed a self-consistent foreign-witness negative: both complete
  domain/current-key endpoints pass, while configured startup refuses the
  foreign authority before any managed write or witness request. R129-568
  rejected an actual signed Final-only intent/settlement addition at the
  Original authority gate; its deletion control was insufficient because its
  manually assembled guard could reject earlier, so that command is partial.
  R129-569 passed the corrected full native guard: unmodified Original is
  accepted, and actual Apply refuses settlement deletion at the exact frozen
  authority-advance boundary with zero writes/requests. These results do not
  authorize publication completion, UNKNOWN conversion, mixed post-witness
  histories, current fact authority, or joint A-D/full 9.8a acceptance.

  R129-571 reproduced the next native commit_selection_durable root gate after
  real Observe, selection creation and exact readback, before commit signing.
  Both Original and Final now resolve the selection's exact previous/committed
  bundles and stored request/observation from their complete independently
  validated graphs. Exact selection validation reconstructs the unsigned commit
  and witness binding without signing; both post-witness cuts retain the
  independent Advance receipt check. R129-572 passed two offline native starts
  (runtimeapp 142.009s), preserving the selection, exchange and missing suffix.
  R129-573 passed actual full-guard Apply rejection of the selected observation's
  deletion and signed Final-only selection/exchange negatives, each at its exact
  authority gate with zero managed writes or witness requests; committed-cut
  deletion, signed Final-only advance and foreign-witness controls also passed
  (combined runtimeapp 47.410s). Existing associated preservation retains every
  Original exchange while allowing valid independent additions. These are two
  single-attempt historical cuts only; later/mixed prefixes, current fact
  authority and joint A-D/full 9.8a remain open.

  R129-574 retained a fixture-only commit readback-address failure; corrected
  native RecordDigest readback in 575 reached the commit_receipt_durable root
  reconciliation RED. This third single-attempt cut now retains exact selection
  witness validation and independent Advance verification. Existing Preflight
  requires the complete unsigned commit to equal selection.UnsignedCommit.
  R129-576 passed two offline native starts (runtimeapp 140.259s), and 577 passed
  full-guard commit deletion and signed Final-only commit negatives at their
  exact gates without managed writes or witness requests (15.861s).
  R129-578 then passed the actual independent-thread HTTP lifecycle with the
  witness disabled before initial startup: create_plan, protected-source
  boundary, real ordinary read marker, four local synthetic Provider requests,
  persistence, shutdown and fresh-restart identical history (158.496s).
  Original report execution/history/usage stayed explicitly denied, the complete
  report/advance and original exchange members stayed unchanged, and corrupt
  ordinary Core still failed before writing. Two failed SharedWitness requests
  occurred across the new boundaries, with zero successful Observe/Advance/
  Resolve, no Risk requests and no restart retry. Failure precedes endpoint
  dispatch in this fixture, so counters alone do not identify the failed request
  endpoints; their Observe origin is supported by the production call chain.
  This extends the focused ordinary-continuity evidence to a durable committed
  report during an outage, not later/mixed prefixes, current Fact admission,
  real external Provider/Electron execution, or full A-D/9.8a/Product/Formal RC.

  R129-580 retained a test build failure from a nonexistent decision validator;
  corrected native decision creation/readback in 581 reached the root gate at
  delivery_decision_durable, before Core result persistence. R129-582 passed two
  offline starts for that cut but did not establish same-state settled-Core
  qualification. R129-583 was a fixture syntax failure. R129-584 reproduced the
  actual first-preservation-gate overqualification: both matching and foreign
  admission Core results were accepted without a grant settlement. R129-585
  narrowed qualification to the full current replay's Active grant and passed
  two before-result starts plus both settled-Core refusals (153.799s). R129-586
  passed all four native Active-prefix first gates and actual Apply decision
  deletion / signed Final-only decision controls (25.741s).

  The matching Core-result-before-GrantSettlement cut now reuses pendingwork's
  pure durable result audit through an Original-only entrypoint. It verifies the
  current-installation receipt, full Core replay, exact private host admission,
  decision-bound Active registry and real Active-to-Settled transition. The
  primary comes from the existing held scope and its complete byte hash is
  revalidated; no settlement is synthesized or signed. Live held-thread and
  disposition refusals remain. R129-587 passed pendingwork (1.345s), two offline
  native starts retaining the matching Original result and missing suffix
  (138.62s), and foreign/missing admission first-gate plus complete-root refusals
  (14.73s), without Original writes or witness requests. R129-588 was a launcher
  shell parse failure and executed no tests. Independent source review identified
  a detached API nil-authority panic path; the entrypoint now rejects that state.
  R129-589 passed pendingwork again (1.326s), actual full guarded Apply result
  removal refusal (4.65s), and a genuine signed Final-only matching-result program
  rejected by fresh startup before writing (8.04s). Existing whole-primary
  preservation owns this boundary; no redundant result map was added. These are
  exact historical single-attempt cuts only. Later grant-settlement/disposition/
  completion prefixes, mixed histories, current Fact authority and joint A-D /
  full 9.8a remain open; no Product/Formal RC claim.

  R129-591 reproduced the next exact grant_settlement_durable root gate after
  native current-key creation/readback and before disposition. That state now
  enters the same independent selection/exchange and Advance checks. Its actual
  Core result must retain private admission even though Preflight already
  validates the persisted settlement's signature and real transition. R129-592
  passed two offline starts (138.61s), foreign/missing admission with a signed
  settlement matching the actual result digest (14.81s), full guarded settlement
  deletion (4.34s), and signed Final-only settlement rejection (10.96s). R129-593
  additionally checks the same Original inspection result against the persisted
  settlement using the independent current key, with an explicit Settled
  requirement. Four earlier Active-prefix gates and the exact settlement/admission
  controls passed (41.326s). No missing settlement or disposition is synthesized.

  R129-594 reproduced both native completed pending disposition and durable
  StageCompletion prefixes before DeliveryOutcome (11.593s). These already-closed
  Core effects now have a distinct single-attempt historical qualification,
  complete Original/Final plan and full4 equality, exact pending pairs, successful
  result/settlement/completion graph, and independent Advance/selection witness
  checks. They reuse the existing completed semantic guard and frozen publication
  route, without setting the failed/cancelled closedPreWitness flag or holding
  the whole thread. R129-595 reached a later settlement reconciliation Observe
  failure (51.247s); its combined write/request assertion does not distinguish
  actual managed writes. The existing complete Original registry/settlement
  qualification now also includes these two closed successful prefixes.
  R129-596 passed two offline native starts for each (107.19s and 108.37s), with
  exact Original pending/full4/advance and Core graph, no whole-thread hold and
  no witness retry on the second start. First-start attempt counts were not
  independently asserted in that command.

  R129-597 passed actual same-thread ordinary HTTP read with SharedWitness
  offline before first startup: two local synthetic Provider calls, persistence,
  shutdown, fresh restart and identical history without Provider/witness replay
  (112.70s). The logged cumulative witness count of 10 includes fixture setup;
  it is not ten runtime requests. Both closed prefixes accepted a harmless
  primary rewrite while actual full guarded Apply refused original result and
  disposition deletion (17.65s). Actual StageCompletion deletion and a genuine
  signed Final-only StageCompletion program were refused before managed writes
  (10.81s). Missing delivery remains missing; no report Apply, release, current
  Fact admission or outcome authority is supplied. These native single-attempt
  cuts do not cover unsettled/superseded intent histories, mixed histories, all
  public A-D consumers, or full 9.8a/Product/Formal RC.


  R129-598 passed earlier Active-prefix, closed UNKNOWN/foreign authority/Final
  erasure and completed-result regression gates (38.076s), Windows compile-only,
  strict OpenSpec and diff checks. R129-599 cancelled the intent fixture before
  its secure CAS authority write completed and failed fixture readback; it was
  not a startup RED. R129-600 instead cancelled only after the native intent
  store returned successfully, verified exact durable intent with no settlement
  and zero witness calls in that fixture, then reproduced the global root gate
  (6.152s). A missing settlement alone does not establish absence of external
  witness effects.

  The single-attempt unsettled intent now receives preservation-only startup
  qualification: independent installation/current key/witness anchors, complete
  actual Original previous/next bundles and publication index binding, Active
  Core grant, pending HMAC and full Original/Final equality. It does not retry
  witness work, infer settlement or synthesize a terminal suffix. R129-601 was
  an unused test local compile failure and executed no native tests. R129-602
  passed two offline native starts (135.81s), foreign witness refusal (5.37s),
  actual guarded intent deletion / signed Final-only intent controls (13.56s),
  and absent / signed Final-only next-bundle controls (15.12s); runtime suite
  170.717s. Superseded/blocked and mixed histories, current Fact authority and
  joint A-D/full 9.8a remain open.

  R129-603 additionally passed Windows runtimeapp compile-only, strict OpenSpec
  and diff checks for the unsettled-intent candidate. The next native route is
  committed-before-selection with an already persisted UNKNOWN disposition:
  the actual pending service CloseAllOpenOnRestart produces that status from
  its complete Active Core grant. R129-604 was a test field compile failure;
  R129-605 exposed an obsolete empty seed shard in the synthetic receipt
  replacement fixture. Removing only that exact empty fixture directory allowed
  R129-606 to reproduce the global startup gate (6.315s).

  This single DeliveryBlocked cut now retains the existing whole-thread hold,
  exact signed UNKNOWN and absent suffix. It requires complete Original/Final
  equality, actual previous/next bundles and index, independent committed Advance,
  stage HMAC and Active Core replay. R129-607 passed two offline native starts
  preserving all protected records and the primary without witness retry
  (136.63s; runtimeapp 137.441s). R129-608 passed the fresh UNKNOWN and older
  committed first gates (4.85s), actual guarded UNKNOWN deletion refusal (3.96s),
  absent / signed Final-only next-bundle refusals (14.74s), and pendingwork's
  existing unresolved-effect refusals (0.508s). That command was PARTIAL: its
  Final-only UNKNOWN test stopped at an earlier Core gate, while its timestamp
  replacement cancellation fixture did not establish the intended cut.
  R129-609 corrected those observations and passed genuine signed Final-only
  UNKNOWN addition (3.43s) and current-key re-signed DisposedAt replacement
  (2.92s), both rejected by the original unresolved Core authority gate before
  managed writes or witness requests. These proofs grant no closing, retry,
  current Fact or delivery authority. Mixed report histories and remaining
  A-D/full 9.8a stay OPEN; Superseded has a domain constructor but no current
  Coordinator producer, so CAS conflict is not claimed as native supersession.

  2026-09-09 R129 complete-inventory candidate (base `cb3c9cbe5`, uncommitted):
  the Original/Final master inventory and orphan audit now precede per-attempt
  closed/deferred proof partitioning. All-open, all-closed, mixed closed/open,
  and multiple turns on one thread retain the same complete authority
  denominator. Any unresolved attempt holds its entire thread, including closed
  history on that primary; independent closed threads can perform ordinary work.
  Actual disposition-overlaid prefixes retain their stored UNKNOWN/Failed
  records: UNKNOWN requires an Active Core grant, while post-intent Failed
  requires its actual failed Core result. Every present intent and selection
  still binds independent Original witness evidence. No missing suffix, retry,
  UNKNOWN, current Fact admission, or delivery authority is synthesized.

  R129-616/617/618 passed complete partition and actual guarded Apply controls;
  R129-619 passed a five-attempt mixed native HTTP/tool/two-offline-start sample
  (269.62s). R129-621 passed actual restart-produced disposition prefixes and
  retained authority refusal controls. R129-624 passed failed-result erasure
  refusal before Apply and signed UNKNOWN versus settled Core refusal; its
  CaseProject positive expectation was invalid because case public summaries
  intentionally omit private workspace. R129-625 corrected that test without
  changing public projection and passed held request/detail/usage refusal,
  independent case thread listing, private CaseProject membership exclusion,
  actual ordinary read, persistence and two offline starts (224.68s), preserving
  report/pending/held-primary records without Provider or witness replay.
  R129-626 passed the complete pendingwork, executiongrant, thread, usage,
  casethread and HTTP adapter packages; focused server turn/fork/resume/usage
  refusal controls; Windows runtimeapp compile-only; strict OpenSpec and diff
  checks. Windows execution and formal packaged evidence were not performed.
  R129-622/623 executed no tests due to launcher/selection errors. These remain
  synthetic native and loopback-Provider worktree results; merged Owner review,
  full 9.8a and downstream A0/B1 acceptance remain open. Superseded has no current
  Coordinator producer and is not introduced by this change.

  2026-09-09 R129 rev14 correction (same base, uncommitted): Owner declined
  the rev13 candidate and closed review round1 with F1/F2. R129-627 reproduced
  swallowed one-shot native CaseEntity/EvidenceRegistry EIO; R129-632 reproduced
  real signed fork/resume history blocking a newly appended report. Both RED
  receipts and the earlier candidate remain historical and are not overwritten.

  CaseEntity/EvidenceRegistry now separate pure in-memory domain errors from
  native storage/control failures through the existing typed classification.
  Only an exact domain-only error may isolate; native EIO, cancellation and
  mixed/unknown access errors retain their original cause before journal
  preparation. Direct CaseEntity/registry activation uses the same boundary.
  Inherited case-derived history now remains inside the whole-primary hold and
  turn-ID floor without entering the execution-context set. Qualification
  requires complete current-key Original lineage, a terminal user-only history
  shape, and absence from all Original/Final staged or committed contexts,
  pending/child allocations, child job and auto-continue execution identities.
  Signed lineage binds the derivation, not the provenance of historical text.
  No context is re-signed and no execution, retry or report authority is added.

  R129-634 passed missing/malformed/wrong-turn context, unexpected authority,
  forged/missing Original lineage, and actual interrupted early semantic
  observation including Final-only lineage refusal. R129-636 passed actual
  fork/resume scope, recursive child scope and starting/failed continuation
  reservation component refusals. R129-637, on the fixed candidate, passed
  CaseEntity/EvidenceRegistry pure-domain ordinary work/restart, native access
  error refusal with zero journal preparation/unchanged bytes, and the real
  derived-report/independent ordinary HTTP read with two offline starts and no
  Provider or witness replay. R129-638 passed scoped owner/consumer regression
  tests and Windows runtimeapp compile-only; no Windows execution was performed.

  R129-630 executed no test because of launcher quoting. R129-631 retained a
  stale report-record ordinary-activation expectation failure; the complete
  inventory gate requires refusal for an unattributable committed attempt, now
  asserted with unchanged-state evidence in R129-634. R129-635 passed the
  native lifecycle but its recursive-child assertion compared different JSON
  number representations; R129-636 checks the exact persisted primary SHA.
  These are bounded synthetic native/loopback results. New Owner review round2
  and integration are pending; full 9.8a, A0/B1, Product and Formal RC remain
  unaccepted. No stage, commit, package, release or external publication occurred.

  2026-09-09 R129 rev15 correction (same base, uncommitted): Owner closed
  round2, accepted the focused F1 fix and retained F2 for actual compacted
  derived history. R129-639 reproduced the missing projection whitelist via
  real PrepareCaseCompaction/CommitCompaction -> ForkThread/ResumeSession.
  The validator now accepts only the producer's three closed inert shapes,
  retains exact historical numbers and bytes, and still rejects context
  erasure, unknown fields and execution authority. Original signed lineage,
  complete Original/Final execution-identity exclusion, child/auto-continue
  reservations and held turn-ID floors remain required.

  The already sealed inherited proof now reaches eventlog preservation,
  final inventory and metadata-only public thread projection. It requires
  the exact Original turn and unchanged whole-primary SHA; it cannot sign
  contexts or authorize execution. Every final/thread remains inventoried;
  current compaction, default public projection and SSE retain signed gates.
  R129-648 reproduced the strict JSON-number comparison failure; 649/650
  exposed the eventlog consumer and its numeric decoder mismatch. R129-655
  passed startup but failed public listing; 656 reproduced that public
  compaction misclassification and 657 passed the bounded projection fix.

  R129-659 passed complete pendingwork/thread/casethread/eventlog packages,
  actual fork/resume with authority_only_v1, compaction_authority_v1 and
  user_only_untrusted_v1, 17 compacted-history and eight ordinary-history
  refusals, early Original/Final journal observation, recursive child and
  auto-continue controls, lossless values, sealed consumer/public refusals,
  and Windows runtimeapp compile-only. R129-660 passed the affected native
  fixture with real V2 terminal finals and actual compaction, a new held
  report, independent ordinary HTTP read, exactly two loopback Provider
  calls and two offline starts without witness replay. Native compaction
  retains protected final turns plus the real compaction boundary; the
  authority skeleton and user-tail producer shapes are separately proved
  by the fork/resume component fixtures, not claimed as native full finals.

  Intermediate 641/643-647/651/653 failures remain recorded. Source fixture
  terminal authority and epoch-start snapshot omissions were corrected;
  652 proved final CAS unchanged through compaction, 653 exposed required
  startup context repair, and 654 passed the repaired fixed-point check.
  658 passed the four packages and compacted boundary controls but two
  fixtures collided with a nonempty pending receipt shard; cleanup now
  removes only a confirmed empty seed directory. Windows did not execute
  in 658. No failed command is relabeled as passed. Rev15 round3 review and
  integration are pending; full 9.8a, A0/B1, Product and Formal RC remain
  unaccepted. No stage, commit, package, release or live-user-data access.

<a id="snapshot-d469-case-tasks-1828"></a>

### case-tasks: original lines 1828–1966

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `23d195a56fd48a72e79a6703344082e1cb9bec2f1a60d21e05698ab365bcac30`.

  2026-08-16 fresh non-completion evidence: the single formal A0 run of the
  `e95a58a6a616656e22e34ee8928b333d2d7c50df` package reached the real visible
  Hub-authenticated Provider path, but failed correctly at the Plan result gate
  because one Provider response contained two `create_plan` calls and the
  second failed after the first effect. The post-run repair now rejects that
  complete duplicate batch before persistence or execution, removes its
  arguments from retry history/events, and requests one bounded exactly-once
  retry. Focused loop/HTTP/A0-harness tests plus fresh typecheck, lint, build,
  ordinary, prod-tag, and race matrices passed; this does not upgrade the
  failed artifact, authorize another live run, satisfy B1 formal inputs, or
  close 9.11/10.x.

  The first clean-HEAD release-gate attempt later failed its deterministic
  speed/cache child because the performance command silently selected the
  retained `e95a58a6a616656e22e34ee8928b333d2d7c50df` packaged binary whenever
  `dist/` existed, instead of the current source commit used by the historical
  deterministic baseline. The repair keeps the `<=250ms` threshold and all
  durable admission/publication rules unchanged, makes the default gate build
  the current `analytix_prod` Go source, and reserves packaged execution for an
  explicit app/binary selection or live-package probe. The focused contract
  and fresh 19-command speed/cache slice passed with first safe progress at
  `193ms`. The parent turn-start path also reuses the exact store-returned,
  transition-writer-protected post-barrier view/digest pair, validates its
  identity, workspace, status, and expected digest, and retains the final
  durable append CAS and persisted readback. Three additional isolated source
  samples passed at `171ms`, `198ms`, and `203ms`, and the complete 79-test
  release-contract file passed. This repair does not upgrade the
  failed A0 artifact, provide B1 authority, refresh live evidence, or close
  9.11/10.x.

  The 2026-08-17 DOCX source-freeze Level 3 run exposed a new deterministic
  failure at the unchanged `<=250ms` first-safe-progress gate: independent
  samples were `258ms` and `264ms`. Stage timing localized the delay to the
  event-log candidate and journal-temp durability flushes, not authority
  derivation, provider execution, or the 16ms renderer batch. The repair keeps
  both file syncs, the journal rename, both required directory frontiers, every
  crash cut, durable append CAS, and `turn_started` publication order, but
  waits for the two independent pre-authority file flushes concurrently. The
  complete event-log adapter package and the exact public performance contract
  passed; a fresh source-built prod-tag report passed at `238ms` with no
  Provider draft, reasoning, credential, or raw-value publication. This is a
  focused regression repair only. The first post-repair full Vitest run then
  reached `4797/4798` and exposed one source-contract assertion still requiring
  `json.NewEncoder(temp)`; the production encoder now deliberately writes the
  same temp file through `io.MultiWriter(temp, candidateDigest)`. The focused
  contract now requires that digest-bound writer and passed. Neither result
  completes the final Level 3 or authorizes package/formal reuse.

  The first `fe6f109a29662e59fda9e9ee0653e733552f7ebe` packaged A0 actual
  attempt used the source-bound arm64 artifact with SHA-256
  `375d048abde1ebbfd426b8bdf8dbdc854e062417eddfe98494f068baca209af6`.
  Visible Computer Use completed the normal fresh Hub login, the configured
  Provider completed all eight logical/physical calls, the parent executed the
  exact task tool once, the source repair and parent-owned repository test
  passed, and the report retained no credential or raw Provider body. It still
  failed before continuation because the public Go thread-summary omitted the
  durable child `maxModelSteps`/`timeBudgetMs` and allowed a newer lifecycle
  projection to erase profile/policy metadata, while the harness required three
  completion markers that its own workflow prompt never requested. The failed
  report SHA-256 is
  `7d8aa6b19b3fdb029a9103097e8ef73a606a9773cb32c4e1bf054e200cce6ab3`.
  The candidate repair adds only closed non-negative execution-bound metadata,
  restores immutable job-owned lineage/profile/policy/bounds after event merge,
  and aligns the prompt with the unchanged marker gate. Focused Go public
  projection/contract tests, the TypeScript public schema test, and all 173 A0
  harness tests passed. This evidence does not upgrade the failed artifact,
  authorize reuse of its pair, complete a new source freeze, or close 9.11.

  The first post-repair ordinary Go module matrix then passed every package
  except one root shard: under unrelated sustained host storage load, the
  production-command test was scheduled as shard 219 and its nested runtime
  did not emit readiness inside the unchanged 60-second product boundary. The
  exact test passed without a timeout change. The root coordinator now places
  every registered fixed-deadline full-stack shard in its first worker wave
  before interleaving ordinary shards, while retaining nine workers,
  one-test-process isolation, child `GOMAXPROCS`, and every product/coordinator
  timeout. Focused scheduler/product checks and the complete 319-shard root
  package passed, the latter in `767.363s`. The failed module matrix remains a
  failed attempt and a fresh final Level 3 is still required before packaging.

  The first `aafd6714f2f534bcbcb965d7d715789481279140` packaged A0 actual
  attempt used the source-bound arm64 DMG with SHA-256
  `cd5b232936fa774f5cbcf606a7e64226e350611de784a62105ba7627dea2e9b6`.
  Visible Computer Use completed the normal fresh Hub login; the exact Plan,
  repository mutation and parent-owned test, bounded read-only subagent,
  ordinary MCP, research/writing markers, protected-funds fail-closed turn,
  and bounded continuation Provider turn all completed with successful
  receipts. The attempt then failed closed at `long-context-continuation`
  before compaction or relaunch because the A0 harness still required the
  historical `analytix.host-final-renderer/v1`, while the production Go V5
  current-write contract emits `analytix.host-final-renderer/v2`. The failed
  report SHA-256 is
  `09fcfcbd5f6a0f8d43e6eef0fb1aa9b00efbbf009d66197497e1c9c8171907a9`.
  The candidate repair changes only the current A0 authority check and fixture
  to V2 and adds a negative V1 regression; all 173 focused A0 harness tests
  passed. This does not upgrade or rerun the failed pair, complete a new source
  freeze, authorize B1 actual on the failed artifact, or close 9.11.

  The single `5c6e1f36252d3fc1caede1a45f3b82e92fadb726` packaged A0 actual
  attempt used the source-bound arm64 DMG with SHA-256
  `da529fad9044d15127a5506355c55fd7d15d53f524cfdc765a1186912576e3e8`.
  Visible Computer Use completed the normal fresh Hub login, and the exact
  Plan, repository mutation and parent-owned test, bounded subagent, ordinary
  MCP/research/writing, and protected-funds fail-closed stages passed. The
  attempt then failed closed at `long-context-continuation` with
  `continuation_turn_invalid`: the current turn was terminally projected as
  `case_terminal_provider_failure` before any bound Provider attempt
  (`providerAttemptCount=0`), while the required safe failure diagnostic was
  absent (`provider_failure_diagnostic_missing`). The failed report SHA-256 is
  `950947e27b1b777c5abfbe335467190f825da8c3f66951df5c6114fc2f68028c`.
  No credential or raw Provider body was recorded, and no reasoning-markup or
  identity-challenge code was observed. This evidence is insufficient to
  distinguish model/profile drift, pre-Provider admission, and host
  publication failure, so the pair was not retried and no speculative gate or
  product change was made. B1 actual, current-commit live evidence, and the
  release gate were not run; 9.11 remains open.

  The 2026-08-18 deterministic public-seam reproduction resolved that
  ambiguity without reusing the failed artifact or calling a Provider. The
  production HTTP path accumulated two large ordinary accepted turns, sixteen
  protected source-unavailable case turns, and one small ordinary
  continuation. Before the repair it performed zero compactions: preflight
  estimated the raw thread projection below the soft threshold while the
  actual typed ordinary provider history still contained old accepted prose,
  so the exact request reached the hard guard before Provider admission. The
  red/green production-Go test
  `TestMixedCaseLongContextHTTPPublicSeamCompactsBeforeOrdinaryContinuation`
  now proves one trusted automatic compaction, a third Provider request, and
  accepted final publication without old ordinary prose or protected case
  prompts. `TestContextHardLimitHTTPPublicSeamEmitsHostAdmissionDiagnostic`
  separately proves zero Provider calls and the closed PII-free
  `provider_admission_rejected` projection containing only reason code,
  projected request tokens, hard-threshold tokens, and attempt count. The
  implementation reuses the existing signed case-compaction authority and
  typed continuation; it adds no authority, registry, CAS, or protocol
  version. This deterministic evidence closes the observed pre-Provider root
  cause but does not satisfy the still-required real packaged case
  continuation, credentialed A0, authorized B1 input, or current live-evidence
  receipt, so 5.10k and 9.11 remain open.

<a id="snapshot-d469-case-tasks-1979"></a>

### case-tasks: original lines 1979–2017

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `d886c1fb843fb9f2ddef1dcc59c3c1eac2e211aef45bb12fd9f0311f9e57d833`.

  The final frozen-source TypeScript checks passed: typecheck in `17.35s`,
  lint in `21.00s` with zero errors and seven existing warnings, build in
  `12.60s` including its `build:runtime` dependency, and the runtime package
  suite at `1122/1122` in `50.96s`. The first full root Vitest run reached
  `4797/4798` but one unchanged `<=250ms` APFS durability sample measured
  `275ms`; the exact focused test and direct source-built performance gate then
  passed at `239ms`, no threshold/timeout/source change was made, and a fresh
  complete root rerun passed `4798/4798` in `376.17s`. This isolated scheduling
  fluctuation remains reported rather than hidden.

  The first 2026-08-17 ordinary matrix passed `318/319` root shards but one
  shard failed to observe its nested production runtime ready line: nine
  ordinary child processes each inherited all 12 host CPUs, allowing 108
  runnable Ps despite outer `-p=1`. The exact product test passed independently
  in `8.971s`. The scheduler repair retains nine workers, one-test processes,
  explicit caller policy, and every timeout, but divides implicit ordinary
  `GOMAXPROCS` across the active workers (`2` on this host); the established
  race one-third cap is unchanged. Focused scheduler/product checks passed and
  the complete ordinary root inventory then passed in `781.385s`. The final
  frozen-source module commands subsequently passed without test-result reuse:
  ordinary in `1786.18s`, `analytix_prod` in `1025.31s`, and race in
  `2525.68s`; the race root completed in `870.882s`, below the unchanged
  per-package budget, with no race finding. A sourced `gofmt -l` inventory over
  every existing changed/untracked Go file emitted no path.

  The 2026-08-18 post-root-cause candidate repeated the complete requested
  source matrix without weakening a threshold, timeout, test inventory, hard
  guard, or Final Gate. Typecheck, lint (zero errors and seven existing
  warnings), and build passed. Root Vitest passed `466` files and `4799` tests
  in `339.16s`; the public runtime package passed `106` files and `1123` tests
  in `44.14s`. The serialized ordinary, `analytix_prod`, and race Go module
  matrices all passed with `-count=1`; representative final packages were
  ordinary root `607.274s`, runtimeapp `227.612s`, server `276.602s`;
  `analytix_prod` runtimeapp `225.893s`, server `257.930s`; and race root
  `713.886s`, runtimeapp `205.252s`, server `484.954s`. The architecture gate
  also passed in all three matrices after moving typed ordinary-history
  orchestration out of the server facade. These are deterministic source
  checks only; release-gate, package, Electron, A0, B1, and live evidence keep
  their independent states below.

<a id="snapshot-d469-case-tasks-2030"></a>

### case-tasks: original lines 2030–2038

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `4c7fa9dcef892d171c3bc50d98c9d2f5b419cf8472e843c8b29fafb89f390fba`.

  The 2026-08-18 candidate control-plane slice built one prod-tag binary and
  passed its threshold-free structural checks, two warmups, and twenty
  measured samples on the accepted `Mac16,8`/Apple M4 Pro/12-CPU host. The
  measured samples were `192, 234, 226, 213, 202, 235, 174, 224, 220, 234,
  238, 228, 236, 247, 192, 215, 220, 220, 243, 185` ms; nearest-rank p50 was
  `220ms`, p95 was `243ms`, and max was `247ms`. This candidate run occurred
  before the task files were committed, so it is cross-layer evidence only and
  cannot replace the single source-freeze release-gate receipt required for
  final control-plane acceptance.

<a id="snapshot-d469-case-tasks-2041"></a>

### case-tasks: original lines 2041–2117

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `021dcea0a4d71d5eaf02e14c4a3c974abe4ae87c47f9997d97e188612657e6a2`.

  The 2026-08-22 Slice 8 candidate adds a release-mode-only closed ingestion
  contract for the existing packaged Milestone A and B reports. It accepts
  only the two fixed report names, revalidates their current harness/source,
  independent A0/B1 lane, exact packaged authority, staged payload, app.asar,
  executable, runtime-server, and Funds-plugin bindings, and records their
  report hashes in a same-run normalized receipt. Missing, partial, duplicate,
  cross-source, cross-artifact, symlink/hard-link, drifting, or
  non-pass evidence remains fail closed. Exact artifact legal admission stays
  owned by the artifact legal audit. Any artifact-scoped third-party
  commercial-license decision, plus signing, notarization, publication, and
  release authorization, stays separate from engineering admission; this does
  not override the Apache-2.0 product-license settlement recorded below. This
  focused synthetic contract evidence is not a
  formal package/Electron/A0/B1 run, does not close any remaining RC row, and
  leaves 9.11, 9.11c, 10.4, and all dependent rows unchecked.

  The Slice 8 follow-up closes the PASS/root-blocker projections, requires a
  clean packaged-worktree snapshot, and performs release-mode formal-evidence
  preflight before the one-run receipt is created. Final admission reuses the
  packaged-build authority verifier and re-inspects the whole exact artifact
  through the legal-audit owner after selected-component verification. Focused
  synthetic mutation and subprocess tests passed; no formal run, package,
  Electron, A0/B1, Provider, or external release authorization was executed.
  Evidence remains reusable for the same immutable source, harness, and exact
  artifact without an arbitrary elapsed-time cutoff; out-of-tolerance future
  timestamps still fail closed. The open rows above remain unchanged.

  The second Slice 8 follow-up rejects reporter-inconsistent nested PASS
  projections for the admission-critical A0/B1 operator, repository, Provider,
  workflow/recovery, typed-local, public-scan, exit, and cleanup sentinels while
  leaving B1 post-RC joint-case work deferred and commercial publication
  authority separate. Malformed packaged authority objects now produce closed
  ingestion failures instead of throwing. The existing after-pack authority
  owner and exact-artifact legal owner require single-link stable app.asar and
  packaged-root reads and compare the directory/symlink inventory before and
  after the point-in-time inspection. Focused synthetic mutation, hard-link,
  and deterministic interleave checks passed; no formal gate, package,
  Electron, Provider, A0/B1, signing, notarization, publication, or release
  action was run, and all open formal rows remain unchanged.

  On 2026-08-23 the Owner selected Apache-2.0 for Analytix, its public runtime
  package, and the bundled first-party Funds plugin. The canonical Apache text
  and copyright are carried only by the product `LICENSE`; existing actual
  third-party obligations remain separate in `THIRD_PARTY_NOTICES.md`, with no
  new root NOTICE and no upstream research or private-permission narrative in
  the product license. The source legal plan and hostile metadata fixtures must
  pass before this settlement is committed. The follow-up binds source root,
  runtime, Funds, and root/runtime lock metadata, then independently rejects
  exact-artifact root/runtime/Funds license drift, including the production
  `app.asar.unpacked` runtime-metadata layout. Exact-artifact legal inventory,
  signing, notarization, publication, release authorization, 9.11/9.11c, and
  10.4/10.5 remain independently open; the license decision is not a formal
  artifact or release PASS.

  The 2026-08-24 Slice 19 candidate closes the upstream artifact-admission
  projection consumed by the release gate. The upstream audit now binds the
  exact release HEAD/tree, tracked manifest and provenance inputs, manifest
  ancestor, artifact-entering rows, and pinned source/license objects into a
  deterministic receipt hash. The control plane requires that closed
  projection and its source bindings but keeps research-checkout freshness
  diagnostic; product RC additionally requires the independent upstream
  admission gate. The current Kun-derived baseline therefore remains an exact
  operational blocker, while exact packaged-artifact legal admission and all
  commercial, signing, notarization, publication, and release authority stay
  independently owned and false. This focused implementation does not execute
  or close 10.4 or 10.5.

  The Slice 19 P1/P2 follow-up keeps that ownership split while closing five
  fail-open or unstable-input seams found in independent review. Artifact
  admission now classifies exact pinned license bytes through a closed content
  fingerprint set, rejects declared/detected class mismatch and non-root
  license paths, requires exactly one complete Current Review Queue, disables
  Git replace refs for release/provenance object reads, preserves exact queue
  item text, and projects malformed entry-problem shapes as stable blockers
  instead of throwing. This adds no authorization registry or legal grant;
  Kun remains blocked and research freshness remains diagnostic-only. It does
  not execute or close 10.4 or 10.5.

<a id="snapshot-d469-case-tasks-2120"></a>

### case-tasks: original lines 2120–2131

Source: `openspec/changes/case-evidence-publication-gate/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `77e66df38b7ee1d8fdf52c26ee9f000b0f4d51fc96b379e93d0570dd104f281a`.

  On 2026-08-17 the current candidate passed `git diff --check`, strict OpenSpec
  validation, tracked-link review, the ignored plan-artifact check, and the
  focused product-sovereignty/security contracts. The former four
  high-severity production findings are now
  removed from the actual dependency graph: `html-to-docx` and its bundled
  `image-size` parser are absent, and the affected `nanoid`/`js-yaml`
  resolutions were upgraded without an override. Fresh production and full
  dependency audits both report zero high/critical findings. Nine moderate
  findings remain in the no-fixed-version `file-type` path retained by the
  existing computer-use/Jimp chain and are a separately recorded residual
  risk. Task 10.5 remains open until the frozen-source scan set and package
  inventory are complete.

The `case-tasks-1` block was byte-identical in both active tasks files at d469;
its single historical owner above preserves both origins.

<a id="snapshot-d469-brand-tasks-69"></a>

### brand-tasks: original lines 69–145

Source: `openspec/changes/define-agent-platform-brand-architecture/tasks.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `24ccb70e92750fee36b305dd8e19ddb26a631f64d978d10d6ade388937f5f0f6`.

## Focused implementation ledger

- 2026-08-26 Slice 31 `CANDIDATE_RESULT` closes only the raw Vision image-provider bypass sub-denominator of task 3.2: a package-private trusted-local projector seam and the Core-owned final provider projection now precede every Vision Bridge `Provider.Stream`. Because production has no trusted projector or positive derivative admission, missing/rejected/failed projection and uninspectable `Data`/`DataBase64`/`ImageURL`/data URL inputs are quarantined with a value-free image-effect unavailable result; focused actual-runtime evidence records bridge image calls `0`, primary image-bearing calls `0`, one independent primary text call, and no open attachment receipt. Broad task 3.2 remains unchecked; positive trusted projection and every other 3.2 lane remain separate work.
- 2026-08-26 Slice 32 `CANDIDATE_RESULT` closes only the `analytix serve` ready/startup metadata sub-denominator of task 3.3: both machine and formatted stdout now reuse one frozen positive projection containing only `service`, `mode`, and the validated bound `port`; configured paths, model/runtime policy, process identity, timestamps, and unrestricted metadata are omitted, while insecure mode retains a value-free warning. A compiled-process hostile-path canary, the runtime package suite, and the Electron process consumer test pass. Broad task 3.3 remains unchecked; later runtime logs, telemetry, export, diagnostics, crash, support-bundle, and other public/durable lanes remain separate work.
- 2026-08-26 Slice 32R `CANDIDATE_RESULT` closes only the raw child-stdout bypass residual within the `analytix serve` ready/startup metadata sub-denominator of task 3.3: pre-ready non-marker stdout is suppressed, the internal `ANALYTIX_RUNTIME_SERVER_READY` marker is consumed, and post-ready child stdout is drained without forwarding, so parent stdout remains only the two serializations of the same frozen positive `service`/`mode`/validated `port` projection. Focused hostile pre/post tests, a compiled-process exact stdout/stderr canary, the full runtime package suite, the Electron process consumer, package/root typechecks, the runtime build, and strict OpenSpec validation pass. Child stderr/log policy and the READY-after-immediate-SIGTERM race are unchanged separate lanes; broad task 3.3 remains unchecked, and all other public/durable lanes remain separate work.
- 2026-08-26 Slice 33 `CANDIDATE_RESULT` closes only the raw Go child-stderr bypass within the `analytix serve` startup/diagnostic sub-denominator of task 3.3: child stderr is continuously drained and discarded before and after the internal ready marker, and a pre-ready nonzero exit preserves code/signal while omitting the raw tail or excerpt and emits no count/digest diagnostic. Focused hostile pre/post/exit tests, the full runtime package suite, the Electron process consumer, package/root typechecks, the runtime build, focused ESLint, and a compiled-process exact stdout/stderr canary pass with child path/PII/model/PID/timestamp sentinels absent. Broad task 3.3 remains unchecked; Go stderr producers, live Electron adapter behavior, global logs, telemetry, export, crash/support bundles, the READY-after-immediate-SIGTERM race, and all other public/durable lanes remain separate work.
- 2026-08-26 Slice 34 `CANDIDATE_RESULT` closes only the Electron `runtime:restart` startup-error public-projection sub-denominator of task 3.3: after the existing Funds-cleaning revocation, any private restart rejection is replaced at the positive `ipcMain.handle('runtime:restart')` boundary by the existing closed `runtime_unavailable` RuntimeError code and canonical host-authored message, while a successful restart still resolves with `undefined`. Serialized handler and renderer IPC-style hostile path/PII/model/PID/timestamp/secret canaries pass with the existing safe localized summary, Agents settings action, and value-free renderer detail. Broad task 3.3 remains unchecked; internal digest-only logging and all other public/durable lanes remain separate work.
- 2026-08-26 Slice 35 `CANDIDATE_RESULT` closes only the enabled `diagnostics:thread-trace` final durable-writer sub-denominator of task 3.3: after untrusted IPC parsing, the main-owned sink replaces renderer-controlled `threadId` with one deterministic SHA-256 value-free ref before filename selection and JSONL serialization, admits numeric/boolean/null data only through an event-specific positive key allowlist, and returns the existing `{ ok, path }` result shape with a safe relative trace path instead of absolute `userData`. Focused append/flush/file/result and IPC hostile path/PII/metadata tests, batching, 20 MiB rotation, global pruning, an isolated synthetic compiled-service canary, root typecheck/build, focused ESLint, diff checking, and strict OpenSpec validation pass. Broad task 3.3 remains unchecked; Go producers, other logs/telemetry/export/crash/support lanes, timestamp policy, and diagnostics UX remain separate work.
- 2026-08-26 Slice 36 `CANDIDATE_RESULT` closes only the Write HTML/PDF/DOC/rich-clipboard local-image read-containment sub-denominator of task 3.3: the final Main-owned image inliner now requires the trusted active Write workspace root, canonicalizes each local image before reading, and rejects outside absolute paths and workspace symlink escapes with one fixed value-free result and zero target-artifact or clipboard publication. `write:copy-rich-text` preserves its existing payload shape but now requires the current main frame, derives active-workspace authority from Main settings, ignores renderer `workspaceRoot` as authority, and rechecks that authority before clipboard publication; a genuine workspace-local image still embeds, and the existing ordinary PII/reasoning/accepted-final projections remain unchanged. The two focused suites pass `70/70`, the isolated durable HTML canary passes `2/2`, and root typecheck/build, focused ESLint, and strict OpenSpec validation pass. Broad task 3.3 remains unchecked; DOCX is unchanged, and raw HTML/external links/base or font paths/target-path projection, image signatures/limits/SVG sanitization, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 37 `CANDIDATE_RESULT` closes only the saved-project durable HTML source-directory base-href sub-denominator of task 3.3: `exportWriteDocument(format=html)` now constructs the final artifact without the Main-authored source-directory `<base href="file://...">`, while ordinary public content, the existing success result shape, and the default HTML document behavior used by PDF/DOC remain unchanged. A focused real-export RED→GREEN canary writes and rereads the artifact with a sentinel present only in the synthetic source directory; the final bytes retain the ordinary content and omit both the base element and sentinel. The canary passes `1/1`, the full focused suite passes `21/21`, and root typecheck/build, focused ESLint, diff checking, and strict validation of both active OpenSpec changes pass. Broad task 3.3 remains unchecked; raw HTML, relative/external link and resource behavior, font URLs, target-path projection, PDF/DOC/DOCX/rich clipboard, images, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 38 `CANDIDATE_RESULT` closes only the saved-project durable HTML relative-link private-path projection sub-denominator of task 3.3: the final `exportWriteDocument(format=html)` anchor renderer now preserves Markdown relative link `./linked-note.md` as the exact safe relative href instead of publishing a source-directory `file://` anchor, while ordinary link text/content, the existing success result shape, and Slice 37's omitted `<base>` behavior remain unchanged. Images retain their existing resolution/inlining path; the default helper/PDF/DOC/rich-clipboard paths retain existing relative-resource resolution, and DOCX remains on its independent document model. A real durable write/read canary first failed with `fileHrefPresent=true` and `sourceDirectorySentinelPresent=true`, then passed `1/1`; the full focused suite passes `22/22`, and root typecheck/build, focused ESLint, diff checking, and strict validation of both active OpenSpec changes pass. Broad task 3.3 remains unchecked; external links, font URLs, raw HTML, images and their residuals, target-path projection, PDF/DOC/DOCX/rich-clipboard artifact proof, shared contracts, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 39 `CANDIDATE_RESULT` closes only the saved-project durable HTML official-font private-resource-path projection sub-denominator of task 3.3: final `exportWriteDocument(format=html)` artifacts now embed the two fixed official WOFF2 files as `data:font/woff2;base64,...` sources instead of publishing absolute packaged `file://` URLs, while the default HTML helper and therefore existing PDF/DOC font loading retain their file-source behavior; DOCX and rich clipboard remain on independent paths. An isolated cache-root export→write→read probe and the permanent canonical canary both failed with `fileFontUrlPresent=true` and `resourcePathSentinelPresent=true`, then passed `1/1`; the full focused suite passes `23/23`, and root typecheck/build, focused ESLint, and diff checking pass. Broad task 3.3 remains unchecked; external links, raw HTML, images and their residuals, target-path projection, PDF/DOC/DOCX/rich-clipboard artifact proof, font compatibility, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 40 `CANDIDATE_RESULT` closes only the saved-project durable HTML POSIX-absolute Markdown-anchor private-path projection sub-denominator of task 3.3: final `exportWriteDocument(format=html)` artifacts now preserve the public link text but omit the `href` when the anchor target is a POSIX-absolute path, including a fragment-bearing target, while an exact safe relative link and a normal `https:` external link remain unchanged; the default HTML helper and therefore existing PDF/DOC/rich-clipboard link resolution remain unchanged, and DOCX remains on its independent document model. An isolated cache-root export→write→read probe first failed with `fileHrefPresent=true` and `privatePathSentinelPresent=true`; the permanent canonical canary reproduced that RED and then challenged the first candidate with `absoluteFragmentHrefPresent=true` before both bare and fragment-bearing cases passed. The full focused suite passes `24/24`, and root typecheck/build, focused ESLint, diff checking, and strict validation of both active OpenSpec changes pass. Broad task 3.3 remains unchecked; Windows/UNC/direct-file-URL anchor classifications, other external-link classes, raw HTML, images and their residuals, target-path projection, PDF/DOC/DOCX/rich-clipboard artifact proof, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 41 `CANDIDATE_RESULT` closes only the saved-project durable HTML direct-`file:` Markdown-anchor private-path projection sub-denominator of task 3.3 with a baseline-GREEN canary and no product-source change: the existing Markdown URL sanitizer removes the hostile `file:///private/...#fragment` target before final rendering, so the durable artifact preserves ordinary content and public link text while omitting both the `file://` href and private-path sentinel; a normal `https:` external link remains exact. An isolated cache-root export→write→read probe and the new permanent canonical hostile canary each pass `1/1` on their first run; the full focused suite passes `25/25`, and root typecheck/build, focused ESLint, diff checking, and strict validation of both active OpenSpec changes pass. Broad task 3.3 remains unchecked; Windows/UNC anchor classifications, protocol-relative and other external-link classes, raw HTML, images and their residuals, target-path projection, PDF/DOC/DOCX/rich-clipboard artifact proof, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 42 `CANDIDATE_RESULT` closes only the saved-project durable HTML Windows-drive and slash-authority Markdown-anchor projection sub-denominator of task 3.3: a `C:/...` target remains baseline-GREEN under the existing Markdown scheme sanitizer, while final `exportWriteDocument(format=html)` artifacts now preserve link text but omit every `//authority/path` href because a standalone `file:` document would otherwise treat it as a local file authority. A normal explicit `https:` link and Slice 38's safe relative link remain exact; the default HTML helper and therefore existing PDF/DOC/rich-clipboard resolution remain unchanged, and DOCX remains independent. The isolated cache-root artifact probe and permanent canary reproduced `uncPathSentinelPresent=true` before passing `1/1`; the full focused suite passes `26/26`, and root typecheck/build, focused ESLint, diff checking, and strict validation of both active OpenSpec changes pass. Broad task 3.3 remains unchecked; backslash-escaped UNC source normalization, other external-link classes, raw HTML, images and their residuals, target-path projection, PDF/DOC/DOCX/rich-clipboard artifact proof, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 45 `CANDIDATE_RESULT` closes only task 1.6: the canonical desktop/default package, shortcut, and uninstall display identity is `Analytix`, while package name, bundle executable, CLI/protocol/app id, environment prefix, and the explicitly pinned Electron `appData/analytix` directory retain lowercase machine identity; the official Standard Windows `Analytix灵鉴` display exception remains unchanged. Focused app/config tests pass `46/46`, runtime conformance passes `22/22`, root typecheck/build pass, and a fresh cache-root macOS package reads back `CFBundleName=Analytix`, `CFBundleDisplayName=Analytix`, `CFBundleExecutable=analytix`, `CFBundleIdentifier=com.analytix.desktop`, plus asar package identity `{name: analytix, productName: Analytix}`. The package authority remains explicitly `development_dirty_non_publishable` with `releaseEligible=false`; this task acceptance is not Product, release, or Formal RC acceptance.
- 2026-08-26 Slice 46 `CANDIDATE_RESULT` closes only the saved-project durable HTML backslash-UNC Markdown-anchor projection sub-denominator of task 3.3: CommonMark normalizes a typical `\\server\share\...` target into an active `%5Cserver%5Cshare%5C...` href, so the final HTML anchor projector now rejects literal or leading percent-encoded backslash absolute targets before admitting safe relative links. The public link text, ordinary content, exact safe relative href, and normal `https:` external href remain unchanged. A real export→write→read canary first failed with the private-path sentinel in the active encoded href and then passed `1/1`; the full focused suite passes `27/27`, and root typecheck/build and focused ESLint pass. Broad task 3.3 remains unchecked; raw HTML remains a reserved content-semantics decision, and images, other external-link classes, target-path projection, PDF/DOC/DOCX/rich clipboard, other collectors, package, and release evidence remain separate work.
- 2026-08-26 Slice 47 `CANDIDATE_RESULT` closes only the existing Main managed-log final-writer and shared public-console projection sub-denominator of task 3.3 with baseline-GREEN evidence and no product-source change: caller-controlled message/category text and arbitrary detail fields are replaced by fixed categories/event codes plus allowlisted nonnegative numbers, booleans, and full SHA-256 values; raw or malformed managed-child records are rejected, and hostile property enumeration or getters fall back without publishing their values. The real temporary-log write→read and public-console suite passes `5/5` with hostile PII/path/UNC/SQL/raw-tool/private-authority/reasoning/credential canaries absent, and focused ESLint and diff checking pass. Broad task 3.3 remains unchecked; direct console sites outside this projector, Go logs, telemetry, renderer diagnostics, timestamp policy, crash reports, support bundles, package, and release evidence remain separate work.
- 2026-08-26 Slice 48 `CANDIDATE_RESULT` closes only the TypeScript runtime `HybridThreadStore` direct-console projection sub-denominator of task 3.3: SQLite fallback and metadata-compaction warnings now publish fixed value-free event codes instead of caller action text, raw error text, filesystem paths, or thread identifiers, while SQLite failure fallback/rethrow behavior, JSONL persistence, compaction retry, and store control flow remain unchanged. A constructor-failure canary first reproduced a private-path sentinel in the SQLite warning, and a real oversized metadata append with a hostile thread/path sentinel exercises the compaction catch; both exact projection canaries pass `2/2`, the full hybrid-store suite passes `15/15`, and runtime/root typechecks and builds plus focused ESLint pass. Broad task 3.3 remains unchecked; `context-compactor`, `file-session-store`, `goal-resume-coordinator`, other direct console sites, Go logs, telemetry, renderer diagnostics, timestamp policy, crash reports, support bundles, package, and release evidence remain separate work.
- 2026-08-26 Slice 49 `CANDIDATE_RESULT` closes only the TypeScript runtime `FileSessionStore` usage-compaction direct-console projection sub-denominator of task 3.3: a best-effort compaction rejection now publishes one fixed value-free warning retaining the existing conformance prefix instead of the thread identifier, raw error text, or filesystem path, while the already-appended event, append-only fallback, cache invalidation behavior, and compaction control flow remain unchanged. A real mocked atomic-write rejection first reproduced hostile thread/path sentinels in the warning and then passed with the appended event sequence still exactly `[1, 2, 3]`; the full file-session-store suite passes `6/6`, the focused G5 source-conformance test passes `1/1`, and runtime/root typechecks and builds plus focused ESLint pass. Broad task 3.3 remains unchecked; `context-compactor`, `goal-resume-coordinator`, other direct console sites, Go logs, telemetry, renderer diagnostics, timestamp policy, crash reports, support bundles, package, and release evidence remain separate work.
- 2026-08-26 Slice 51 `CANDIDATE_RESULT` closes only the TypeScript shadow/test-support `ContextCompactor` inflated-token direct-console projection sub-denominator of task 3.3: an untrusted report beyond the configured trust factor still falls back to the local estimate with `shouldCompact=false` and `planCompaction=null`, while the rate-limited warning now emits one fixed value-free event code instead of the model value or dynamic reported/estimated token counts. A focused RED canary first reproduced its private-path model sentinel and both dynamic counts in the warning, then passed `1/1`; the full loop suite passes `114/114`, the runtime package suite passes `1137/1137`, and runtime/root typechecks and builds plus focused ESLint pass. Broad task 3.3 remains unchecked; production Go runtime, other direct console sites, Go logs, telemetry, renderer diagnostics, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 52 `CANDIDATE_RESULT` closes only the TypeScript shadow/test-support `GoalResumeCoordinator` fallback direct-console projection sub-denominator of task 3.3: startup and scheduled-launch failures now emit distinct fixed value-free event codes instead of thread identifiers or raw errors when no injected logger exists, while startup still returns `false` and the scheduled path retains its delay and launch attempt; the existing injected internal logger message contract remains unchanged. Focused RED canaries first reproduced hostile private-path thread and error sentinels on both fallback paths, then passed `2/2`; the runtime package suite passes `1139/1139`, and runtime/root typechecks and builds plus focused ESLint pass. Broad task 3.3 remains unchecked; production Go runtime, injected logger projection, other direct console sites, Go logs, telemetry, renderer diagnostics, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 53 `CANDIDATE_RESULT` closes only the TypeScript model-test-support `CompatModelClient` HTTP-failure direct-console projection sub-denominator of task 3.3: both terminal HTTP-failure call sites now emit one fixed value-free event code instead of request/configured models, provider response bodies, base/request URLs, status, or endpoint metadata, while request execution, retry classification, and the existing public 404 configuration-hint error chunk remain unchanged. A focused RED canary first reproduced hostile private-path model and base-URL sentinels after query-secret redaction, then passed `1/1`; the full model-client suite passes `62/62`, the runtime package suite passes `1139/1139`, and runtime/root typechecks and builds plus focused ESLint pass. Broad task 3.3 remains unchecked; public/provider error projection, model requests, `ThreadService` and its conformance fixture, other console/log/telemetry/diagnostic lanes, production Go runtime, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 54 `CANDIDATE_RESULT` closes only the TypeScript shadow/test-support `ThreadService` goal-persistence direct-console projection sub-denominator of task 3.3: persistence failure now emits the exact fixed value-free event `[analytix] event=ANALYTIX_GOAL_PERSISTENCE_FAILED` without the hostile thread ID, original error, or action marker, while the original `Error` object still surfaces unchanged to the caller. A focused RED canary first reproduced all three dynamic values in the warning, then passed `1/1`; the full ThreadService and G5 TypeScript conformance files pass `18/18` and `22/22`, the goal-specific Go replay matches the synchronized G5 fixture, Go internal conformance compiles and passes, the runtime package suite passes `1139/1139`, and runtime/root typechecks and builds, focused ESLint, fixture parsing, and strict OpenSpec validation pass. The pre-existing aggregate Go control-executable test remains separately red at package-runtime-identity ProductName drift before this Slice assertion and its oracle was not weakened. Broad task 3.3 remains unchecked; production Go logs, other console/log/telemetry/renderer diagnostic lanes, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 55 `CANDIDATE_RESULT` closes only the production Go `cmd/runtime-server` top-level fatal-stderr projection sub-denominator of task 3.3: a fatal startup error now emits the exact fixed value-free event `[analytix] event=ANALYTIX_RUNTIME_SERVER_FAILED` instead of a timestamped raw error while retaining exit code `1`. A real subprocess RED canary first reproduced a hostile private-path sentinel passed as an invalid `--port` value in stderr and then passed with exact event-only bytes; the shared-data-root, shared-durable-root, and nested-root lease conflict tests now assert the same fixed projection and retain causal strength by proving the same arguments start successfully after the active holder is released. The focused fatal/lease group, the full `cmd/runtime-server` package, `go vet`, the command build, diff checking, protected-state hashes, and strict OpenSpec validation pass. Broad task 3.3 remains unchecked; the shutdown-drain warning, internal/server stderr producers, the TypeScript launcher drain, public HTTP errors, telemetry, renderer diagnostics, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 56 `CANDIDATE_RESULT` closes only the production Go `cmd/runtime-server` failed shutdown-drain retry-stderr projection sub-denominator of task 3.3: each failed bounded drain attempt now emits the exact fixed value-free event `[analytix] event=ANALYTIX_RUNTIME_SHUTDOWN_DRAIN_RETRY` instead of a timestamped raw lifecycle error, while the process retains its persistence lease, retries, releases the lease only after a successful drain, and returns the original serve failure. The existing lifecycle/lease test first reproduced a hostile private-path shutdown-error sentinel on the real retry path and then passed with exact event-only stderr bytes; the focused canary, full `cmd/runtime-server` package, `go vet`, command build, diff checking, and strict OpenSpec validation pass. Broad task 3.3 remains unchecked; internal/server stderr producers, public HTTP errors, telemetry, renderer diagnostics, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 57 `CANDIDATE_RESULT` closes only the production Go `recordRuntimeBestEffortEvent` RecordEvent-failure stderr projection sub-denominator of task 3.3: a failed best-effort event append now emits the exact fixed value-free event `[analytix] event=ANALYTIX_RUNTIME_EVENT_RECORD_FAILED` instead of caller context, thread/turn identifiers, or the raw persistence error. A real `DurableEventSessionStore.beforeRecordEventHook` RED canary first reproduced a private-path context, the store-owned thread identifier, a hostile turn identifier, and a private-path error in stderr, then passed with exact event-only bytes while retaining one record attempt and an unchanged event sequence. The focused canary, full `internal/server` package, server vet/build, diff checking, protected-state hashes, and strict OpenSpec validation pass. Broad task 3.3 remains unchecked; the bounded background-lifecycle diagnostic, other turn/restore/server stderr producers, public HTTP errors, telemetry, renderer diagnostics, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 58 `CANDIDATE_RESULT` closes only the production Go `completeRuntimeTurnAsync` terminal-persistence-failure stderr projection sub-denominator of task 3.3: after a real async provider failure, failure-terminal persistence rejection now emits the exact fixed value-free event `[analytix] event=ANALYTIX_RUNTIME_ASYNC_TURN_FAILURE_RECORD_FAILED` instead of the thread/turn identifiers or raw persistence error. The existing turn-start terminal-guard seam first reproduced hostile phone-like thread/turn identifiers and a private-path append error in stderr, then passed with exact event-only bytes while retaining the real async start, one persistence attempt, durable `failed` terminal CAS/publication metadata, the recoverable terminal-event gap, idle-thread restoration, and operation/cancellation cleanup. The three focused terminal-guard tests, full `internal/server` package, server vet, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; other server stderr producers, public schemas, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 59 `CANDIDATE_RESULT` closes only the TypeScript `analytix serve` top-level startup-rejection stderr projection sub-denominator of task 3.3: a rejected startup now emits the exact fixed value-free event `[analytix] event=ANALYTIX_SERVE_STARTUP_FAILED` instead of `String(error)`, while the runtime exit code remains `70` and startup/child shutdown control flow is unchanged. A real rebuilt CLI subprocess with an isolated fake runtime first reproduced the hostile invalid-ready path prefix `"/private/c"...` in stderr, then passed with empty stdout, exact event-only stderr, the same exit code, and no private-path bytes. The focused launcher suite passes `23/23`, the runtime package suite passes `1140/1140`, and runtime typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; serve crash handlers, parse/config diagnostics, other server stderr producers, public schemas, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 60 `CANDIDATE_RESULT` closes only the TypeScript `analytix serve` uncaught-exception/unhandled-rejection crash-handler stderr projection sub-denominator of task 3.3: a post-ready crash now emits the exact fixed value-free event `[analytix] event=ANALYTIX_SERVE_CRASHED` instead of the dynamic crash kind, raw error, or stack, while the single-crash guard, bounded close deadline, child shutdown, and runtime exit code `70` remain unchanged. A real rebuilt parent CLI first published the positive ready payload, then a Node `--import` trigger reproduced a complete hostile private-path error and stack through `unhandledRejection`; after the fix it retained both ready serializations and the bound port, closed the isolated fake runtime, emitted only the fixed event, and exposed no sentinel. The focused launcher suite passes `24/24`, the runtime package suite passes `1141/1141`, and runtime typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; parse/config diagnostics, other server stderr producers, public schemas, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 61 `CANDIDATE_RESULT` closes only the Electron Main `runtimeErrorPublicDiagnosticV1` hostile-classification-failure sub-denominator of task 3.3: arbitrary thrown values retain the existing `{ errorBytes, errorSha256 }` opaque public shape, while a throwing `instanceof`/object-classification path now fails closed to the fixed value-free text `unavailable` before those two fields are derived. A production-projector RED canary first showed a hostile `Symbol.toStringTag` getter throwing a complete private-path sentinel through the diagnostic call itself, then passed with the fixed byte count and full SHA-256, no sentinel, and unchanged ordinary Error/string/object behavior. The focused projector/logger group passes `8/8`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; diagnostic schema changes, renderer diagnostics, other logs/telemetry/crash/support-bundle lanes, other server stderr producers, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 62 `CANDIDATE_RESULT` closes only the current Electron Main UI-plugin invalid-manifest parse-error projection sub-denominator of task 3.3: `installUiPluginFromDirectory` retains the existing `{ ok: false, errors: string[] }` result contract, while a JSON parse failure now returns the fixed value-free message `manifest.json 不是合法 JSON` instead of the runtime parser message and echoed manifest bytes. A real isolated source-directory install RED first reproduced the hostile private-path prefix `"/private/c"...` in the generic renderer-facing error array, then passed with the exact fixed message, no sentinel, and no created plugin root; valid allowlist copy, manifest normalization, figure limits, traversal rejection, list/load/remove, and bundled compatibility behavior remain green. The focused service suite passes `9/9`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; task 2 plugin identity/lifecycle/foundation, other plugin errors, public schema changes, other logs/telemetry/diagnostic lanes, other server stderr producers, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 63 `CANDIDATE_RESULT` closes only the current Electron Main UI-plugin load traversal-error projection sub-denominator of task 3.3: `loadUiPluginFigures` retains the existing `{ ok: false, error: string }` result contract, while a plugin identifier rejected by the existing confined-path guard now returns the fixed value-free message `插件标识无效` instead of the guard message and renderer-controlled identifier. The real load seam RED first reproduced the hostile traversal/private-path sentinel `../private/customer-pii-13900000014` in the returned error, then passed with the exact fixed message and no sentinel; successful figure loading and the existing install/list/remove behavior remain unchanged. The focused service suite passes `9/9`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; task 2 plugin identity/lifecycle/foundation, other plugin errors, public schema changes, other logs/telemetry/diagnostic lanes, other server stderr producers, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 64 `CANDIDATE_RESULT` closes only the current Electron Main UI-plugin manifest-structure validation projection sub-denominator of task 3.3: the shared manifest validator retains its detailed internal diagnostics, while `readManifestAt` now converts every validator rejection crossing the Main service boundary to the fixed value-free message `manifest.json 内容无效`; the existing missing, size-limit, and JSON-syntax messages remain distinct and fixed, and install/load result schemas are unchanged. A real source-directory install RED first reproduced the hostile unknown-slot key `/private/customer-pii-13900000015` in the renderer-facing `errors[]`, then passed with the exact fixed message, no sentinel, and no created plugin root. The focused service suite passes `9/9`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; task 2 plugin identity/lifecycle/foundation, validator redesign, other plugin errors, public schema changes, other logs/telemetry/diagnostic lanes, other server stderr producers, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 65 `CANDIDATE_RESULT` closes only the current Electron Main UI-plugin install figure-path error projection sub-denominator of task 3.3: a missing or over-limit source figure still reports its allowlisted slot and fixed failure reason through the existing `{ ok: false, errors: string[] }` contract, but no longer interpolates the manifest-provided relative path. A real source-directory install RED first reproduced `img/customer-pii-13900000016.png` in the renderer-facing error, then passed with the exact fixed message `槽位 swim 加载失败:文件不存在`, no sentinel, and no created plugin root; figure validation budgets, fail-before-copy behavior, and successful installation remain unchanged. The focused service suite passes `9/9`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; task 2 plugin identity/lifecycle/foundation, other plugin errors, public schema changes, other logs/telemetry/diagnostic lanes, other server stderr producers, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 66 `CANDIDATE_RESULT` closes only the Electron Main Write infographic image-provider failure projection sub-denominator of task 3.3: after the existing production `ImageGenClient.generate` or `edit` call fails, the renderer-facing result retains its `{ ok: false, message: string }` contract but now returns the fixed value-free message `image generation failed` instead of the raw provider/client error; the separately classified reference-image unsupported response remains unchanged. A real service-adapter RED made one generation attempt and first reproduced `HTTP 400: /private/customer-pii-13900000017 raw-provider-body` in the returned message, then passed with the exact fixed message, no sentinel, and no created output image directory. The focused service suite passes `16/16`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; provider configuration and request policy, Write export/raw HTML/PDF diagnostics, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 67 `CANDIDATE_RESULT` closes only the Electron Main speech-to-text upstream-HTTP failure projection sub-denominator of task 3.3: both MiMo ASR and OpenAI-compatible transcription adapters retain the existing `{ ok: false, message: string }` result contract and positive numeric HTTP status, while `SpeechHttpError` is now projected as `speech provider request failed (HTTP <status>)` instead of publishing the upstream response body; timeout, cancellation, invalid-JSON, network, configuration, and successful transcription behavior remain separately classified and unchanged. A real non-2xx fetch-adapter RED made one request and first reproduced `/private/customer-pii-13900000018 raw-speech-provider-body` inside the returned provider JSON, then passed with the exact status-only message and no sentinel. The focused service suite passes `8/8`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; speech request/model/audio policy, public schema changes, other provider errors, logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 68 `CANDIDATE_RESULT` closes only the Electron Main speech-to-text generic network-failure projection sub-denominator of task 3.3: after excluding the existing HTTP-status, timeout, cancellation, and invalid-JSON classifications, any remaining fetch/network exception now returns the fixed value-free message `speech provider request failed` through the unchanged result contract instead of formatting raw error/cause details. A real throwing-fetch RED made one request and first reproduced `https://speech.example.test/private/customer-pii-13900000019?token=secret` in the renderer-facing message, then passed with the exact fixed message and no sentinel; all successful MiMo/OpenAI adapter behavior remains unchanged. The focused service suite passes `9/9`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; speech request/model/audio policy, public schema changes, other provider errors, logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 69 `CANDIDATE_RESULT` closes only the model-capability-probe durable `errorSummary` projection sub-denominator of task 3.3: the existing provider/model/base-URL/request-URL/status/httpStatus schema and capability-probe control flow remain unchanged, while failure aggregation now admits only the three typed capability status enums and never copies per-request provider bodies or network exception details. Real non-2xx and throwing-fetch RED canaries first reproduced `/private/customer-pii-13900000020 raw-capability-provider-body` and `/private/customer-pii-13900000021?token=secret` in the returned summary, then passed with two requests per case, unchanged `auth_failed`/`failed` status and HTTP 401 metadata, exact status-only summaries, and no sentinels. Source review confirms the Main IPC handler still strips only transient `ok`, `latencyMs`, and `message` before persisting this now-positive `errorSummary` under `runtime.modelCapabilityProbes`. The focused provider-connection suite passes `14/14`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; ordinary `/models` protected-local diagnostics, capability-probe public schema, provider/model request policy, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 70 `CANDIDATE_RESULT` closes only the Electron Main Write DOCX final-write failure projection sub-denominator of task 3.3: an unknown render or filesystem exception crossing `exportWriteDocument` now returns the fixed value-free message `Write export failed.` through the unchanged result contract, while typed Core-authored private-authority, private-reasoning, and local-image rejections retain their existing exact safe messages. A real Save Dialog to a missing hostile-sentinel parent first reproduced the full cache-root target path in the renderer-facing failure, then passed with one dialog call, the exact fixed message, no sentinel, and no target file; the complete focused suite passes `28/28`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; successful export target-path projection, raw HTML, PDF/DOC/DOCX/rich-clipboard artifact proof, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 71 `CANDIDATE_RESULT` closes only the Electron Main Write rich-clipboard publication-failure projection sub-denominator of task 3.3: an unknown `clipboard.write` exception now returns the fixed value-free message `Write rich clipboard copy failed.` through the unchanged result contract, while typed Core-authored private-authority, private-reasoning, and local-image rejections retain their existing exact safe messages. A real clipboard adapter RED first reproduced `/private/customer-pii-13900000023/clipboard-provider` in the renderer-facing result, then passed with one publication attempt, the exact fixed message, and no sentinel; the complete focused suite passes `29/29`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; raw HTML content semantics, clipboard OS-level effect guarantees, successful export target-path projection, PDF/DOC/DOCX/rich-clipboard artifact proof, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 72 `CANDIDATE_RESULT` closes only the current Electron Main UI-plugin install target-filesystem failure projection sub-denominator of task 3.3: after manifest and figure admission, any target `rm`/`mkdir`/`writeFile`/preview exception now returns the fixed value-free `errors: ['插件安装失败']` through the declared install result instead of rejecting the IPC promise with a private user-data path; source validation, allowlist copy, successful installation, and existing partial-effect semantics remain unchanged. A real blocked target-root RED first rejected with `ENOTDIR` and the full cache-root path containing `customer-pii-13900000024`, then passed with the exact fixed result, no sentinel, and no created plugin root; the complete focused suite passes `10/10`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; task 2 plugin identity/lifecycle/foundation, seed/bundled mutation failures, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 73 `CANDIDATE_RESULT` closes only the Electron Main Hub Agent Marketplace remote-catalog failure aggregation sub-denominator of task 3.3: marketplace, Skill catalog, and install-policy request/parse rejections now contribute only the fixed lane messages `marketplace: unavailable`, `skills: unavailable`, and `install-policy: unavailable` to the unchanged renderer-facing sync `errors[]`, while request concurrency, fallback cache selection/write, successful parsing, and sync result semantics remain unchanged. Three real local HTTP responses first caused V8 JSON parse errors to echo hostile `/private/customer-pii-13900000025..27` response prefixes through all three branches, then passed with exactly three requests, the three fixed messages, and no sentinels; the complete focused suite passes `15/15`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; Hub marketplace response admission/schema, plugin materialization and lifecycle, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 74 `CANDIDATE_RESULT` closes only the Electron Main Write infographic output-mutation failure projection sub-denominator of task 3.3: after a successful image-provider response, image-directory resolution, directory creation, output write, canonical-root, or relative-link exceptions now return the fixed value-free message `image output write failed` through the unchanged result contract instead of copying the filesystem error; provider generation, reference-image policy, target selection, write attempt, and successful path fields remain unchanged. A real workspace-local ordinary file occupying a hostile-sentinel `imageDir` first returned `EEXIST` with the full cache-root path containing `customer-pii-13900000028`, then passed after exactly one generation request with the fixed message, no sentinel, and byte-exact preservation of the occupied file; the complete focused suite passes `17/17`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; successful infographic absolute-path projection, output cleanup/atomicity, provider request policy, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 75 `CANDIDATE_RESULT` closes only the Electron Main Telegram bot-token verification failure projection sub-denominator of task 3.3: remote rejection retains the typed `rejected` code and positive numeric HTTP status while returning the fixed message `Telegram rejected the token (HTTP <status>).`, and a generic fetch/network exception retains the typed `network` code while returning the fixed value-free message `Telegram verification request failed.`; token validation, request count, successful identity projection, polling, send, and runtime lifecycle semantics remain unchanged. Real mocked Electron `net.fetch` RED canaries first reproduced `/private/customer-pii-13900000029 raw-telegram-provider-body` and `https://api.telegram.example/private/customer-pii-13900000030?token=secret` through the renderer-facing result, then passed with one request per case, the exact fixed messages, and no sentinels. The complete focused suite passes `5/5`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; Telegram polling/send/download/log lanes, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 76 `CANDIDATE_RESULT` closes only the Electron Main Telegram outbound-message failure public-projection sub-denominator of task 3.3: after the existing HTML send and plain-text fallback both fail, the runtime now returns the fixed value-free message `Telegram message delivery failed.` through the unchanged `{ ok: false, message }` result and `claw:channel:mirror` path instead of copying the final Telegram `description` or network exception. Real `createTelegramRuntime` adapter RED canaries first reproduced `/private/customer-pii-13900000031 raw-telegram-send-provider-body` and `https://api.telegram.example/private/customer-pii-13900000032?token=secret`, then passed with exactly two send attempts per case, HTML `parse_mode` only on the first attempt, channel removal after `stop()`, exact fixed results, and no sentinels. The complete focused suite passes `7/7`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; Telegram polling/download/log lanes, renderer schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 77 `CANDIDATE_RESULT` closes only the Electron Main automatic GUI-update unknown-error state/result projection sub-denominator of task 3.3: the existing HTTP 400/403/404/429/5xx feed classifications, channel, error code, release URL, and update control flow remain unchanged, while an otherwise unclassified updater exception now becomes the fixed channel-only message `Could not check the <channel> update feed. Open the download page instead.` in both pushed `gui:update-state` events and `checkGuiUpdate` results. A mocked production `autoUpdater` event/rejected-check RED first reproduced `/private/customer-pii-13900000033 updater-event-error` and `https://updates.example.test/private/customer-pii-13900000034?token=secret`, then passed with unchanged `unknown` code and `stable` channel plus no sentinels in either state or result. The complete focused suite passes `8/8`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; manual metadata fetch, update download/install failures, feed/package/release qualification, public schema changes, other logs/telemetry/diagnostic lanes, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 78 `CANDIDATE_RESULT` closes only the Electron Main automatic GUI-update download-failure state/result projection sub-denominator of task 3.3: after the existing availability check and one updater download attempt reject, both pushed `gui:update-state` and `downloadGuiUpdate` now retain `download_failed`, current version, last safe update info, and promise cleanup while returning the fixed value-free message `GUI update download failed.` instead of the updater exception. A mocked production updater RED first reproduced `/private/customer-pii-13900000035 updater-download-error`, then passed with exactly one check and one download attempt, the exact fixed state/result, and no sentinel. The complete focused suite passes `9/9`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; install failures, manual metadata fetch, feed/download/package/release qualification, public schema changes, other logs/telemetry/diagnostic lanes, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 79 `CANDIDATE_RESULT` closes only the Electron Main manual GUI-update metadata-fetch exception projection sub-denominator of task 3.3: when the existing platform/signing gate selects the manual metadata path, a manifest network exception now preserves the `unsupported` code, current version, channel, release URL, one-request behavior, and manual-only control flow while appending only the fixed channel message `Could not read GUI update metadata for the <channel> channel.` to the existing unsupported-build explanation. A real mocked manifest GET RED first reproduced `https://updates.example.test/private/customer-pii-13900000036?token=secret`, then passed with exactly one fetch, zero `autoUpdater.checkForUpdates` calls, the fixed value-free result, and no sentinel. The complete focused suite passes `10/10`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; install failures, feed/download/package/release qualification, public schema changes, other logs/telemetry/diagnostic lanes, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 80 `CANDIDATE_RESULT` closes only the Electron Main Chrome Browser Use renderer-diagnostics status projection sub-denominator of task 3.3: the existing raw profile/native-host/connection inspector and its probe/state machine remain available inside Main, while the already-existing `getChromeBrowserUseStatus` IPC/Doctor seam now returns only fixed extension/native-host identities, platform/check time, typed connection/state/status fields, and an empty profile list; user-data/profile/preferences/extension/manifest/registry/host/browser-client paths, manifest-derived values, disable reasons, and raw problems/reasons are omitted without changing the public schema. A real temporary Chrome profile and native-host manifest plus the existing rejecting `probeConnection` seam first reproduced the complete cache-root path and `/private/customer-pii-13900000037/native-host-pipe-error` in the renderer-facing serialization, then passed with one probe and unchanged `disconnected`/`enabled`/`configured`/`failed` semantics and no sentinels. The focused service/IPC/renderer-settings group passes `75/75`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; backend/nut-js Doctor reasons, protected local diagnostic display, Chrome registration/action results, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 81 `CANDIDATE_RESULT` closes only the Electron Main `ComputerUseDoctorResult` backend/adapter failure-text projection residual of task 3.3: unavailable Analytix Computer Use selection, unavailable nut-js fallback, the public backend reason, and an unexpected Chrome diagnostic rejection now retain the existing selected/available and per-check status semantics while emitting four fixed value-free messages instead of backend readiness/fallback reasons, dynamic-import errors, or adapter exceptions. A real `getComputerUseDoctor` call through its existing backend factory, nut-js dynamic import, and Chrome status dependencies first reproduced `/private/customer-pii-13900000038..41` across all four renderer-facing lanes, then passed with unchanged `nut-js` selection, `available=false`, warning/failed/warning check statuses, one backend selection, one Chrome call, exact fixed messages, and no sentinels. The focused Computer Use/Chrome service, IPC, and renderer-settings group passes `76/76`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; protected local diagnostic display, Chrome registration/action results, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 82 `CANDIDATE_RESULT` closes only the Electron Main Chrome Browser Use extension-page action-failure projection sub-denominator of task 3.3: the existing `openChromeBrowserUseExtensionPage` IPC action now resolves Web Store shell rejections and Chrome Settings launcher failures through the unchanged `PathOpenResult` schema with the fixed value-free message `Could not open the Chrome Browser Use extension page.`, instead of rejecting the IPC promise or returning the internal launcher error. A real Electron `shell.openExternal` RED first rejected with `/private/customer-pii-13900000042/open-external-error`, then passed with exactly one call to the unchanged fixed Web Store URL, the exact fixed failure result, and no sentinel; an independent success canary retains one call and `{ ok: true }`. The focused Computer Use/Chrome service, IPC, and renderer-settings group passes `78/78`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; Chrome native-host registration diagnostics, protected local display, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 83 `CANDIDATE_RESULT` closes only the Electron Main Feishu/Lark and WeChat IM QR-start failure projection sub-denominator of task 3.3: the two existing renderer-facing start functions keep their unchanged start-result schema, endpoint/method, request count, QR success behavior, and WeChat missing-bridge product classification, while all other remote, parse, incomplete-response, resolver, and network exceptions now return the provider-fixed messages `Feishu/Lark QR setup failed.` or `WeChat QR setup failed.` instead of copying exception or response text. Real mocked Feishu `action=begin` and WeChat `web.login.start` RED canaries first reproduced `/private/customer-pii-13900000043/feishu-registration-error` and `/private/customer-pii-13900000044/wechat-bridge-error`, then passed with one request per lane, unchanged fixed endpoint/method, exact messages, and no sentinels. The focused platform-install and IPC group passes `57/57`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; Feishu/Lark and WeChat poll-result projection, real login qualification, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 84 `CANDIDATE_RESULT` closes only the Electron Main Feishu/Lark and WeChat IM QR-poll failure projection sub-denominator of task 3.3: the existing renderer-facing poll functions keep their unchanged result schema, request count, Feishu `authorization_pending`/`slow_down` behavior, WeChat missing-bridge classification, terminal cleanup, and successful credential result, while rejected, incomplete, parse, resolver, and network failures now return the fixed value-free messages `Feishu/Lark QR setup status unavailable.`, `WeChat login was not completed.`, or `WeChat QR setup status unavailable.` instead of copying remote or exception text. Real mocked Feishu `action=poll` and WeChat `web.login.wait` RED canaries first reproduced `/private/customer-pii-13900000045/feishu-poll-error`, `/private/customer-pii-13900000046/wechat-poll-message`, and `/private/customer-pii-13900000047/wechat-poll-error`, then passed with two total requests per lane, exact fixed messages, and no sentinels; independent canaries retain exact Feishu pending and WeChat missing-bridge behavior. The focused platform-install and IPC group passes `62/62`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; the existing successful credential/public trust contract, real login qualification, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 84R `EVIDENCE_REPAIR` corrects the fresh-verification record for Slices 83/84 without changing product behavior: root typecheck exposed that the two start-failure `vi.fn` declarations had inferred zero-argument call tuples even though the canaries inspected the real fetch input and options. Their mock signatures now explicitly accept `RequestInfo | URL` and optional `RequestInit`; the same 12 platform-install behavior tests pass unchanged, root typecheck and build now pass from the repaired source, and focused ESLint, diff checking, and protected-state hashes pass. This repair adds no acceptance sub-denominator and does not alter the Slice 83/84 runtime behavior, successful credential/public trust contract, or broad task 3.3 status; task 3.3 remains unchecked.
- 2026-08-26 Slice 85 `CANDIDATE_RESULT` closes only the Electron Main Feishu/Lark Connect Phone mirror failure-result projection sub-denominator of task 3.3: both existing `claw:channel:mirror` IPC names continue through the same `mirrorThreadMessageToIm` result schema, thread-publication authority check, ordinary-text projection, target conversation, and one bridge send, while a rejected Feishu/Lark send now returns the fixed value-free message `Feishu / Lark message delivery failed.` instead of the adapter exception. A real rejecting bridge-send RED first reproduced `/private/customer-pii-13900000048/feishu-mirror-error` in the renderer-facing result, then passed with one send to the unchanged chat, unchanged projected markdown, preserved internal diagnostic, the exact fixed result, and no sentinel. The complete Claw runtime and IPC group passes `95/95`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; WeChat mirror results, Feishu attachment-upload failure replies, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 86 `CANDIDATE_RESULT` closes only the Electron Main Feishu/Lark attachment-upload failure-reply projection sub-denominator of task 3.3: both the direct existing-file and completed agent-file producers keep their existing workspace containment, real file resolution, filename, upload attempt, reply-to fallback, internal failure diagnostic, and final reply options, while the public failure replies now state only that the named attachment upload failed and never append the adapter error. A real temporary workspace file and rejecting bridge RED first completed the confirmation reply, reply-to upload attempt, fallback upload attempt, and then reproduced `/private/customer-pii-13900000049/feishu-file-upload-error` in the fourth outbound message; GREEN preserves that exact four-send order, canonical file source, no pending reaction, and the fixed `我找到了文件 hello.md，但飞书附件上传失败。` reply with no sentinel. The complete Claw runtime and IPC group passes `96/96`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; attachment filename-publication policy, WeChat file replies, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 87 `CANDIDATE_RESULT` closes only the shared Connect Phone turn-start rejection public-result projection sub-denominator of task 3.3: after the existing thread creation, title patch, attachment handling, and 404 missing-thread replacement logic, a final non-OK runtime turn-start response now produces the fixed value-free `Failed to start turn.` message through the unchanged `ClawRunResult` and provider reply paths instead of copying the runtime response body. A real WeChat webhook handler RED created a thread and issued the actual turn POST before reproducing `/private/customer-pii-13900000050/connect-phone-turn-start-error` in the HTTP 500 JSON response; GREEN preserves the POST and status/result schema with the exact fixed message and no sentinel. The complete Claw runtime and IPC group passes `97/97`, including the existing missing-thread retry coverage, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; terminal failed/aborted turn detail, thread-read failures, other provider bridge results, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 88 `BASELINE_GREEN` closes only the Connect Phone terminal-failed turn hostile-detail canary sub-denominator of task 3.3 with no product-source change: the existing shared `sanitizePublicAssistantText` boundary rejects a turn error combining an internal authority reference and `/private/customer-pii-13900000051/connect-phone-terminal-error`, then the existing status-owned fallback returns exactly `Agent turn failed.` rather than historical assistant text or any raw terminal detail. The real WeChat webhook path starts the turn, polls its terminal thread detail, returns the unchanged HTTP 500/result schema, and contains neither sentinel, authority ref, nor prior-turn text. The complete Claw runtime and IPC group passes `97/97`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; terminal-aborted proof, benign provider-safe terminal text policy, thread-read failures, other provider bridge results, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-26 Slice 89 `CANDIDATE_RESULT` closes only the Feishu/Lark streaming double-boundary-send failure-reply projection sub-denominator of task 3.3: the existing streaming turn start, thread-detail fail-close, fixed public boundary, first boundary send, catch fallback send, pending reaction, and outer ordinary reply behavior remain unchanged, while rejection of both boundary sends now returns the fixed user-facing `Sorry, something went wrong while handling your message.` message instead of the second bridge exception. A real `handleFeishuMessage` RED started the streaming turn, attempted the fixed boundary twice, and then reproduced `/private/customer-pii-13900000052/feishu-stream-boundary-error` in the third outbound agent reply; GREEN preserves that exact three-send sequence, reply target/options, and reaction with no sentinel. The complete Claw runtime and IPC group passes `98/98`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; successful SSE streaming qualification, boundary-send delivery guarantees, other provider bridge results, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 90 `CANDIDATE_RESULT` closes only the built-in WeChat loopback `web.login.start` QR-acquisition failure projection sub-denominator of task 3.3: the existing local JSON-RPC server, HTTP 200 transport, error-result schema, one upstream QR request, session insertion boundary, and successful credential path remain unchanged, while upstream fetch failures and incomplete QR responses now cross the RPC boundary only as the fixed value-free message `WeChat QR setup failed.`. A real local bridge server plus native HTTP JSON-RPC RED first reproduced `/private/customer-pii-13900000053/wechat-qr-upstream-error` from the mocked upstream HTTP 500 response, then passed with the exact fixed error result, one request to `/ilink/bot/get_bot_qrcode`, and no sentinel. The focused bridge/platform/IPC group passes `67/67`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; QR polling, successful credential/public trust semantics, real login qualification, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 91 `CANDIDATE_RESULT` closes only the Electron Main automatic GUI-update managed-runtime-cleanup install-failure projection sub-denominator of task 3.3: the existing downloaded/installing transition, one cleanup attempt, pending release-note write wait, `install_failed` code/current version/result schema, pushed error state, and rule that `quitAndInstall` is not called after cleanup rejection remain unchanged, while both public surfaces now use the fixed value-free message `GUI update installation failed.` instead of the cleanup exception. A real `installGuiUpdate` RED first reproduced `/private/customer-pii-13900000054/updater-install-cleanup-error` in its renderer-facing result, then passed with the exact fixed result/state, one cleanup attempt, zero installer calls, and no sentinel in the pushed state. The complete updater suite passes `11/11`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; native installer failures, feed/download/package/release qualification, public schema changes, other logs/telemetry/diagnostic lanes, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 92 `BASELINE_GREEN` closes only the Electron Main automatic GUI-update native installer synchronous-failure canary sub-denominator of task 3.3 with no product-source change: after the existing downloaded/installing transition and one successful managed-runtime cleanup, a throwing `quitAndInstall(false, true)` call retains one installer attempt plus the `install_failed` code/current version/result schema and pushed error state, while the Slice 91 positive boundary returns only `GUI update installation failed.`. The real install seam rejects with `/private/customer-pii-13900000055/updater-native-install-error`, then proves the exact fixed result/state, one cleanup call, one installer call with unchanged arguments, and no sentinel in any pushed bytes. The complete updater suite passes `12/12`, and root typecheck/build, focused ESLint, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; asynchronous native-process/install completion evidence, feed/download/package/release qualification, public schema changes, other logs/telemetry/diagnostic lanes, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 93 `CANDIDATE_RESULT` closes only the built-in WeChat outbound-message upstream-failure result projection sub-denominator of task 3.3: the existing local bridge startup, real persisted test-account resolution, configured-token check, context-token restoration, one `sendmessage` POST, managed internal diagnostic, and result schema remain unchanged, while failure now returns only the fixed value-free message `WeChat message delivery failed.`. A real `sendWeixinBridgeMessage` call starts the loopback bridge before writing one isolated test account, then an upstream HTTP 500 RED first reproduced `/private/customer-pii-13900000056/wechat-send-upstream-error` in the returned result; GREEN preserves one POST to `/ilink/bot/sendmessage`, removes the exact account file in `finally`, and exposes no sentinel. The focused bridge/platform/IPC group passes `68/68`, and root typecheck/build, focused ESLint, isolated-state cleanup, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; successful WeChat delivery, monitor/polling, attachment behavior, real account/login qualification, public schema changes, other logs/telemetry/diagnostic lanes, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 94 `CANDIDATE_RESULT` closes only the production Go turn-steering preflight canonical-thread-read failure response projection sub-denominator of task 3.3: `steerRuntimeTurn` retains HTTP 500, `internal_error`, the pre-authority early return, and an unchanged durable event sequence, while its message is now the fixed value-free `thread state is unavailable` instead of the filesystem error. A real isolated durable store creates a canonical thread and then replaces its exact thread JSON path with a directory; RED first reproduced the cache/durable path, `/customer-pii-13900000057-steer-thread-read/`, thread identity, and `is a directory` detail in the ActionResult body, then GREEN exposes none of them and proves no event append. The target test, complete `internal/server` package, `go vet`, server build, `gofmt`, diff checking, and protected-state hashes pass. Broad task 3.3 remains unchecked; later steering CAS/security failures, interrupt responses, other server responses/stderr, public schema changes, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 95 `CANDIDATE_RESULT` closes only the production Go terminal-turn event-inventory-load failure stderr projection sub-denominator of task 3.3: when the existing durable event inventory is unreadable, `eventsForTerminalTurn` now emits only the fixed value-free event `[analytix] event=ANALYTIX_RUNTIME_TERMINAL_EVENT_INVENTORY_FAILED` instead of the thread identity, turn identity, or raw store error, while returning no events and preserving the existing fail-closed terminal persistence path. The existing real durable-store test now uses hostile durable-root, canonical thread, and turn sentinels, replaces the event log with a directory, and first reproduced the thread/turn sentinels plus internal error classification in stderr; GREEN proves the exact fixed event, no sentinels, the original event-inventory error, thread and turn still `running`, no terminal item, no `generalTerminalPublication`, and zero new event entries. The target test and complete `internal/server` package pass, along with `go vet`, server build, `gofmt`, diff checking, protected-state hashes, and strict OpenSpec validation. Broad task 3.3 remains unchecked; other server stderr, public schemas, the aggregate G5 ProductName baseline, raw HTML, provider protected-local diagnostics, attachment filename publication, successful credential/public-trust semantics, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 96 `CANDIDATE_RESULT` closes only the production Go restart quarantine active-case-thread stderr projection sub-denominator of task 3.3: `restoreRuntimeState` retains the existing case-thread recognition, fail-closed `CanExecute=false` isolation, open-gate reconciliation, and restart skip behavior, while an active quarantined case thread now emits only `[analytix] event=ANALYTIX_RUNTIME_QUARANTINED_ACTIVE_CASE_THREAD` instead of its canonical thread identity. A real durable store, pending-work authority, continuation service, hostile canonical thread/turn identities, and a non-executable case authority first reproduced the hostile thread ID in stderr; GREEN proves the exact fixed event with no sentinels, successful restore, the quarantined thread and turn still `running`, no terminal item, and an unchanged event sequence. The target test and complete `internal/server` package pass, along with `go vet`, server build, `gofmt`, diff checking, protected-state hashes, and strict OpenSpec validation. Broad task 3.3 remains unchecked; pending-gate replay diagnostics, post-terminal goal-lineage diagnostics, other server stderr, public schemas, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 97 `CANDIDATE_RESULT` closes only the production Go post-terminal goal-lineage failure stderr projection sub-denominator of task 3.3: after the existing ordinary turn terminal commit and terminal-authority release, a failed `RecordCompletedTurnChildRun` now emits only `[analytix] event=ANALYTIX_RUNTIME_POST_TERMINAL_GOAL_LINEAGE_FAILED` instead of the canonical thread and turn identities or the producer error, while preserving the best-effort lineage, terminal, recovery, and authority semantics. A real `jobs.Manager` filesystem write failure first reproduced the hostile cache-root path, thread/turn identities, `child-runs`, and OS error in stderr; GREEN proves the exact fixed event with no sentinels, one ordinary Provider call, a completed/idle durable turn, one terminal publication, and zero retained child-run or goal-lineage events. The target test and complete `internal/server` package pass, along with `go vet`, server build, `gofmt`, diff checking, protected-state hashes, and strict validation of both active OpenSpec changes. Broad task 3.3 remains unchecked; pending-gate replay diagnostics, other server stderr, public schemas, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice 98 `CANDIDATE_RESULT` closes only the production Go interrupt pre-authority canonical-thread-read failure public-response projection sub-denominator of task 3.3: `interruptRuntimeTurn` retains HTTP 500, `security_context_invalid`, the existing pre-terminal early return, and zero interrupt authority or durable event effects, while its message is now the fixed value-free `turn security context is unavailable` instead of the filesystem error. A real isolated durable store creates a canonical thread and then replaces its exact `thread.json` path with a directory; RED first reproduced the hostile cache-root path, canonical thread path, and `is a directory` detail in the `ActionResult`, then GREEN exposes none of them and proves an unchanged event sequence with no retained interrupt terminal reservation. The target and related interrupt arbitration/recovery group and complete `internal/server` package pass, along with `go vet`, server build, `gofmt`, diff checking, protected-state hashes, and strict validation of both active OpenSpec changes. Broad task 3.3 remains unchecked; later interrupt quiescence/gate/terminal errors, unreachable interrupt-event stderr, pending-gate replay diagnostics, other server/public schemas, the aggregate G5 ProductName baseline, raw HTML, package, release, Product, and Formal RC evidence remain separate work.
- 2026-08-27 Slice R-2.1.1a `CANDIDATE_RESULT` closes only the Go-private canonical declaration schema-owner sub-denominator of task 2.1.1: `.analytix-plugin/package.json` now has one typed schema-v1 parser/validator and deterministic canonical encoder for package identity, unique contributions, requested capability declarations, lifecycle protocol/entry policy, and portable package-relative paths. The parser reuses the shared bounded strict-JSON owner to reject duplicate, unknown, missing, wrong-case, invalid, and trailing input; canonicalization normalizes set ordering without mutating its caller; the existing materialization filesystem delegates portable-path syntax to this owner while retaining its independent real-root and symlink confinement. A genuine compile RED preceded the focused GREEN; focused and adjacent domain/filesystem suites, focused vet, and the production `runtime-server` build pass. The expanded `go test ./...` gate fails outside this Slice in root legacy/shard, Darwin process/native-component, and architecture checks, while the new package, adjacent adapter, `internal/runtimeapp`, and `internal/server` packages pass. Top-level task 2.1 and nested task 2.1.1 remain unchecked; the real Funds companion, platform projections/parity, Host admission/grants, TS/MCP consumption, migration, lifecycle state machine, third-party executable plugins, package/release, Product, and Formal RC remain separate work.
- 2026-08-27 Slice S-2.1.1b `CANDIDATE_RESULT` closes the remaining real-artifact and production-admission denominator of nested task 2.1.1 only: the first-party Funds source now carries a canonical `.analytix-plugin/package.json` whose package identity, complete current skill/MCP/asset/public-UI inventory, capability requests, and lifecycle fields are exact executable assertions, while `requestedCapabilities` remains declaration-only and mints no grant. The Go materialization filesystem stable-reads, strictly parses, canonicalizes, and verifies every declared contribution against the same regular-file tree before existing-generation, journal, receipt, index, or ready effects; focused RED/GREEN covers missing, non-regular, symlinked, changed-during-read, unknown, duplicate, invalid, unsafe, and missing-contribution declarations plus the existing-generation fast path and ordinary valid Funds materialization. Electron-builder copies the companion, after-pack requires byte identity before writing the complete-tree authority binding, and an isolated `--local-nonpublishable --arch arm64` app-directory build under `/Volumes/AnalytixCache/development-v3/tmp/s211b-package.uPwNZD` contains source-identical companion bytes at SHA-256 `c04f9f19a0483abcf6ddc5624e0d1b0baf78b62cbf82b42b1f601d1276bb05df`; the production Go inspector independently proves its raw/canonical declaration digests and packaged authority tree/file-count/manifest/entrypoint/byte binding. Focused and adjacent Go tests/vet, the runtime-server build, 43 packaging tests, root typecheck, and the nonpublishable package build pass. Nested task 2.1.1 is checked; top-level task 2.1 and tasks 2.1.2-2.1.5 remain unchecked. Projection parity, Host grant/schema work, legacy migration, third-party executable plugins, publication/release, Product, and Formal RC acceptance remain separate work.
- 2026-08-27 Slice Y-R1A `CANDIDATE_RESULT` closes nested task 2.1.2 only: one shared strict source-projection inspector in `after-pack.cjs` reuses the canonical declaration validator, bounded duplicate-rejecting JSON parser, platform/MCP identity and version parity gates, and canonical-root-confined stable regular-file checks for every declared public-UI contribution. Both Electron after-pack and Hub source preparation call that owner; Hub preparation completes it before deriving or creating, replacing, or writing any output root or archive, derives marketplace plugin identity from canonical `packageId`/`packageVersion`, and copies category/interface only from the validated platform manifest. Genuine isolated REDs prove that changing only canonical `packageVersion` from `0.16.16` to `0.16.17` while every platform/MCP projection remained `0.16.16`, or replacing the intermediate `agents/` directory with a symlink to an outside regular `openai.yaml`, formerly let preparation succeed and create both output and archive; GREEN rejects each case before either effect, including the parent-symlink escape as `funds_plugin_public_ui_invalid`, while retaining the final-file symlink rejection. A valid isolated copy produces exact canonical marketplace identity/UI metadata and a real archive whose complete entry inventory is rooted at the canonical `packageId`. The focused packaging suite passes `55/55`; both scripts pass `node --check`; focused ESLint, root typecheck, the adjacent Go `pluginmaterializationfs` package, and strict OpenSpec validation pass. Nested task 2.1.2 is checked; top-level task 2.1 and tasks 2.1.3-2.1.5 remain unchecked. Host admission/grants, TypeScript/MCP admitted-receipt consumption, legacy migration, permission-bit or continuous-adversary policy, publication/release, Product, and Formal RC acceptance remain separate work.
- 2026-08-27 Slice Z-R1 `TASK_2.1.3_FOCUSED_ACCEPTED candidate` closes nested task 2.1.3 only: after strict canonical declaration plus artifact/provenance verification and before data-directory, installation-authority, store, output, or materialization effects, one independent Go Host static admission decision owns supported schema and lifecycle protocol, the first-party package identity, entry policy, a typed capability ceiling, exact provenance anchors, artifact/authority prerequisites, and the existing Ed25519 signing requirement, while it deliberately does not own or copy an exact publisher package version. A complete isolated `0.16.17` Funds artifact is admitted and propagates the declaration identity/version through the existing signed receipt, signed index, ready marker, active path, and installed-declaration binding; over-ceiling capability, unsupported schema or lifecycle, invalid entry policy or first-party identity, and missing trusted provenance are all rejected before any authority, store, output, or materialization effect. Declarations, manifests, markers, package presence, and admission decisions remain requests or evidence rather than grants, and an identity-unbound legacy read-only consumer cannot materialize a generation. Focused domain, application, filesystem, runtime-server, runtimeapp, installation-authority, `analytix_prod`, vet, cache-isolated build, formatting, diff, and strict OpenSpec validation pass. Nested task 2.1.3 is checked; top-level task 2.1 and tasks 2.1.4-2.1.5 remain unchecked. TypeScript/Main/MCP admitted consumption, lifecycle grant/health/revocation, legacy migration, permission-bit or continuous-adversary policy, publication/release, Product, and Formal RC acceptance remain separate work.
- 2026-08-27 Slice Z-R2 `TASK_2.1.4_FOCUSED_ACCEPTED candidate` closes nested task 2.1.4 only: a complete installation-authority-signed `0.16.17` ready binding first failed the production TypeScript materialization seam at its copied `0.16.16` schema literal, while the same admitted installed generation failed the production runtimeapp startup seam before it could reach an exact MCP identity. TypeScript now validates canonical SemVer and derives the exact active generation path from the signed receipt/index/ready identity; Main MCP configuration, skill discovery, and marketplace/UI projection consume the verified installed binding version rather than an independent release literal; runtimeapp binds the current declaration identity and digests into `ResolveActive`, then carries the resolved signed receipt version through the opaque Host Funds spec, in-process MCP identity, source probe, and evidence-read projection. The metadata-only Funds override is removed: self-authored markers, colliding installed generations, ordinary MCP JSON/configuration, private-looking marker/provenance fields, marketplace metadata, and package presence still cannot construct the opaque Host capability or widen its exact read-only catalog, while ordinary signed `0.16.16` behavior remains GREEN. Focused TypeScript/Main/plugin/UI tests pass `93/93`; focused and adjacent Go domain/application/filesystem/runtime-server/runtimeapp/MCP suites, `analytix_prod` runtimeapp/MCP composition, focused vet, cache-isolated runtime-server build, typecheck, ESLint, formatting, diff checking, and strict OpenSpec validation pass. Nested task 2.1.4 is checked; top-level task 2.1 and tasks 2.1.5/2.2 remain open. Legacy-v0 migration, Host lifecycle/grant/health/revocation, package/release, Product, and Formal RC acceptance remain separate work.
- 2026-08-27 Slice Z-R3 `TASK_2.1.5_FOCUSED_ACCEPTED candidate` closes nested task 2.1.5 only: a real filesystem fixture with an existing Ed25519 installation authority, signed receipt/index, canonical install marker, sealed first-party Funds artifact, and no companion declaration first failed the production runtimeapp consumer as `bundled funds host capability is unavailable`. The bounded adapter now recognizes only the frozen `0.16.16` first-party historical version, exact historical skill snapshot, and fixed historical `mcp/server.mjs` entrypoint when the companion is exactly absent, revalidates the packaged and installed complete trees against the trusted receipt/index, and derives the fixed non-persisted schema-v1 historical declaration independently of the current publisher companion. After signed receipt/index validation, canonical and migrated declarations now unconditionally re-enter the existing independent static admission/capability ceiling before exact entrypoint selection and Host Funds startup; an internally consistent signed canonical generation carrying an over-ceiling capability first reached the Host constructor, then GREEN rejects it Funds-only before that effect while ordinary canonical startup remains valid. A separately constructed canonical `0.16.17` publisher declaration with `mcp/future-server.mjs` remains valid while a companionless `0.16.17` artifact is never treated as legacy-v0: tree inspection first enforces exact declaration-to-disabled-MCP projection parity; runtimeapp selects the exact single `analytix_funds` contribution from the strict tree-validated and admitted declaration; and the opaque Host constructor binds `Args` to that declaration-derived clean absolute path while the existing provenance verifier rechecks root confinement, regular-file identity, complete tree, entrypoint/manifest digests, and installation marker. Exact companion absence is verified with `Lstat` before and after legacy projection; a present regular invalid/unknown/duplicate/unsupported companion, directory, symlink, non-regular object, or non-absence I/O failure rejects without fallback, including at package-reader and runtimeapp Host startup seams. Unsupported legacy identity/version, artifact tamper, missing installation authority, spoofed marker, platform capability metadata, and package presence cannot mint or widen authority. Only a non-context Funds-plugin inspection failure or mismatch carries the capability-local sentinel and lets the startup wrapper return no Host and no error so ordinary Agent work continues; first- or post-anchor inspection cancellation/deadline, authority, application-runner, app.asar, runtime-server, anchor, and other package failures remain global startup errors. The materialization command rejects even a recognized legacy artifact before authority/store/output effects, so no third-party executable or package-only bootstrap is admitted. Existing canonical single-edit `0.16.17` parity remains green. Complete `pluginmaterializationfs` and `packagedbuildauthorityfs` packages plus focused runtime-server/runtimeapp/MCP suites, `analytix_prod` canonical-entrypoint and legacy startup canaries, focused vet, cache-isolated runtime-server build, formatting, diff checking, and strict OpenSpec validation pass. Nested task 2.1.5 is checked; top-level task 2.1 remains unchecked pending separate aggregate review, and task 2.2, third-party execution, package/release, Product, and Formal RC remain open.
- 2026-08-27 Controller Epoch AA `TASK_2.1_AGGREGATE_FOCUSED_ACCEPTED` closes only top-level task 2.1 after a separate final-state review of the complete 2.1.1-2.1.5 chain and fresh verification of the combined denominator. The review challenged every remaining production `0.16.16` literal and found one unjustified exported Host-domain duplicate, `pluginmaterialization.PluginVersionV1`, whose only consumers were tests while the accepted `legacy-v0` compatibility rule already had its own frozen owner. Removing that production constant produced a tight compile RED at the first test dependency; four affected suites now own test-only fixture versions, with no replacement production owner and no change to legacy migration, entry policy, evidence-v1 compatibility, or task 2.2 lifecycle. The final aggregate preserves the closed canonical declaration, executable source/platform/MCP/marketplace parity, independent static admission and capability ceiling, exact receipt/index/ready and installed-generation bindings, declaration-derived Host version/entrypoint, strict companion handling, bounded legacy migration, Funds-only local disable, and ordinary Agent availability. Fresh Controller verification passes the complete relevant Go declaration/materialization/packaged-authority/runtimeapp/MCP/runtime-server packages, focused `analytix_prod` Host/Funds canaries, focused vet, a cache-isolated runtime-server build, four TypeScript materialization/packaging/marketplace/skill suites at `92/92`, root typecheck, script syntax checks, formatting/diff checks, and strict OpenSpec validation; the first parallel TypeScript run reached `90/92` only because two I/O-heavy cases exceeded the fixed five-second harness timeout, and the isolated identical assertions passed under a 20-second timeout. Task 2.1 is therefore checked; task 2.2 capability lifecycle, task 2.3 registry, task 2.4 broader isolation, third-party execution, package/release, Product, and Formal RC acceptance remain open.
- 2026-08-27 Slice AA-R2 `TASK_2.2_FOCUSED_ACCEPTED` closes only task 2.2 with one concrete Go-owned capability: exact `funds.source.read` protocol v1 authority for `case:bound` plus `source:verified`, limited to `count_case_rows`. The opaque Host Funds constructor privately clones and re-runs static first-party admission before lifecycle construction, so declaration, package, receipt, marker, marketplace, MCP metadata, and ordinary configuration remain requests or evidence and cannot mint or widen the grant. Host-owned setup/ready/failed/stopped/disabled/revoked transitions bind the current admitted package/spec, server identity, catalog, lifecycle generation, and connection epoch; disconnect, fatal/native failure, provenance or catalog drift, disable, and restart revoke stale authority, while deterministic reconnect repeats admission and issues a fresh generation without changing the stable binding digest. Production manager discovery, schema/identity/epoch accessors, evidence read, and count execution require the current typed grant before and after the native effect; an adversarial blocked native read proves revocation before the final current check keeps the callback effect at zero. Missing or wrong source-read requests disable only count, never authorize `funds.case.read` or account-flow, and preserve ordinary MCP/Agent availability; decision events remain bounded, typed, value-free, and are neither persistent capability-use receipts nor a general audit platform. Independent review closed a candidate-owned fixture race and the evidence-read post-native lifecycle gap; fresh Controller verification passes full MCP race, focused ordinary and `analytix_prod` domain/MCP/runtimeapp suites, ordinary and production-tag vet, the complete runtimeapp package, formatting/diff checks, a cache-isolated runtime-server build, and strict OpenSpec validation. Tasks 2.3-2.5, broader plugin failure isolation, runnable Desktop/CLI proof, package/release, Product, and Formal RC acceptance remain open.

<a id="snapshot-d469-development-373"></a>

### development: original lines 373–408

Source: `docs/analytix/development-baseline.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `82d60ee7a888a74f3f409e3caac0b799b0878bcaa9c681e827d4d4fa1c6c2ccb`.

## Verification snapshot and remaining work

The first [public CI run at e4fe8e0](https://github.com/Eysn0130/analytix/actions/runs/34628860194)
passed source baseline and all **1,617 backend tests**. Application tests
reported **6,088 passed, 59 failed and 15 pre-existing skips**; full Go tests
also failed. These are recorded failures, not release approval. Subsequent
commits must use their own CI results rather than inheriting this snapshot.

That run exposed three misnamed non-Darwin Go files: the `_darwin.go` filename
suffix contradicted their build constraints. Their implementation bytes were
preserved while correcting platform selection, with a dedicated regression
test. This is a compile/source-selection correction, not a new security design
or a claim that Linux native authority and all platform tests are qualified.
The corrected files passed platform-selection tests, the three existing Darwin
path/alias security regressions, and a Linux x64 `analytix_prod` runtime-server
cross-build. Cross-compilation is not Linux runtime or package acceptance.

Remaining work has distinct owners and acceptance criteria:

- Platform-aware test fixtures and source-set assertions: preserve their
  privacy/authority assertions; explicitly model the platform under test.
- Package tests and supply: make resource fixtures self-contained; real
  browser/native resources still need a verified supply route and a qualified
  dedicated builder. Do not replace missing real payloads with empty files.
- Historical formal-evidence tests: reconcile public-baseline fixtures with
  their actual contracts, without importing private history or fabricated
  acceptance receipts.
- Dependency security: the first enabled Dependabot inventory had 78 open
  advisories (33 high, 42 medium, 3 low), often multiple advisories for one
  package. Review minimal compatible fixes with regression tests; enabling
  scanning is not a claim of zero vulnerabilities. CodeQL default setup also
  completed successfully for Actions, Go, JavaScript/TypeScript and Python.
- Full installer/GUI/upgrade acceptance remains unexecuted. The package
  workflow's missing-runner preflight was exercised and correctly rejected
  packaging as `NOT_CONFIGURED`; it did not produce or publish an installer.


<a id="snapshot-d469-development-429"></a>

### development: original lines 429–1152

Source: `docs/analytix/development-baseline.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `a404ecec63750c0b49fd7aefa187a184ae2dbf5c3fda2a64c7a77053c2cc4d99`.

The previous `52b5874` CI finished with successful source/backend jobs and
failed application/default-Go/production-Go jobs. Its application result was
6,094 passed / 53 failed / 15 pre-existing skips. The present changes fix
specific causes (including an unconfigured update-feed fixture, host-dependent
Go source-set assertions and CI private-directory creation); they do not erase
that baseline failure or prove all remaining tests have passed.

The 2026-09-12 stabilization candidate based on `52b5874` was verified locally
on macOS ARM64: 41 baseline tests, native doctor, TypeScript checks, source
build/layout smoke, two deterministic Funds contracts, Darwin/Linux/Windows Go
source-selection tests, and the complete affected Go evidence/reasoning packages
in ordinary and production modes passed. Targeted updater, isolation and
non-disclosure fixture tests passed; these are not a full application-suite
result. A fresh isolated Python 3.11 environment installed from `backend/uv.lock`
passed all **1,617 backend tests** (two dependency deprecation warnings).
Workflow lint passed. Installer and live Provider checks were not executed.
The Git round-trip regression installs the real pre-push hook in both synthetic
clones, including its argv/stdin handling and rejection before remote mutation;
an always-successful hook stub is not evidence of the protected push path.

### PR-mode transition (2026-09-12)

[PR #22](https://github.com/Eysn0130/analytix/pull/22) carries the PR-only hook,
fail-closed aggregate gate and platform-fixture corrections. The public main
remains `58b365b3bc878e987e215d1fb1862ef111a1a37e` until a verified merge.
Local baseline/type/build checks, 18 Git/gate tests and all 15 updater tests
passed. Keychain file-identity and recovery-journal replacement fixtures now
retain the old inode instead of assuming unlink/recreate changes identity.
The case-sensitivity fixture uses distinct alphabetic names and asserts both
filesystem behaviors instead of treating a numeric temporary name as an alias.
These targeted Go checks passed locally in ordinary and `analytix_prod` modes;
the changed Linux branches still require Linux CI. No production permission,
storage or privacy validator was relaxed.

The local full persistence package attempt exceeded a three-minute package
deadline while progressing through startup crash-cut tests: **timeout, not
PASS**. The single previously failing statistics CLI case passed on macOS;
this does not resolve the Linux CI DuckDB internal error. The main CI also
reported application and both full Go-suite failures. Keep this PR draft until
the current candidate's required CI and applicable acceptance actually pass.

The native-filesystem run at `7b8d14d57` established two independent failures:
Linux ext4 fresh private inodes had `FS_EXTENT_FL` (`0x80000`) and no xattrs,
but the validators rejected every nonzero flag; macOS synthetic roots retained
the `/var` ancestor alias, conflicting with strict canonical-path opens. The
corrections recognize only that storage-format bit (all other inode flags and
xattrs remain rejected), and canonicalize the test helper's temporary parent.
The alias regression failed before the helper correction. These changes do not
turn fixture evidence into packaged acceptance; the updated native and full CI
results still decide PR readiness.

At `8fa2265dc`, all five native macOS packages passed in CI and locally; Linux
passed isolation, final-authority, raw-artifact and full persistence (12.29 s).
Its one remaining secure-generation failure was `getdents` returning `ENOENT`
for an already-unlinked held directory. Cleanup now requires exact inode
identity, zero links and platform detachment proof for that case; it never
treats a missing inventory as empty. Same-name replacements remain untouched.
Darwin's ambiguous replacement case continues to fail closed. Parser-only
packaging fixtures now declare resource presence and inject only synthetic
toolchain identity probes, without changing production admission or claiming a
real package build. The separate toolchain contract tests still run.
At `da77ff2fd`, both native filesystem CI jobs passed, including the complete
persistence package; ordinary and production-tag full Go tests remain separate
required checks. Milestone A/B parser tests now use canonical OS temporary
fixtures instead of requiring the Owner's cache mount. A's existing test-module
instrumentation binds only fixture paths and source-relative imports. B's
mocked APFS test explicitly supplies a Darwin platform, with Linux/Windows
rejection tests retained as executable negative coverage. Their local complete
files passed (175 and 34 tests); this is not actual packaged acceptance.
The D-0242 command-availability integration contract requires real macOS
containment. Non-Darwin production adapters intentionally reject protected
process invocation; installing Go on Linux cannot satisfy that contract. The
`macos-integration` Vitest tag routes this unchanged assertion to a required
macOS lane with actual process-sandbox tests. The complementary Linux lane runs
every untagged application test. Neither lane may be omitted from the merge
gate. This classification does not extend the existing test timeout or pretend
that Linux process containment is implemented.
The current-case projector and checkpoint-recovery guards now check the actual
preservation-aware constructor, exact authority arguments, signed journal,
prepared plan/apply pair and scoped callback under the held exclusion. Mutation
tests reject substituted/nil authorities. The focused guards, strict primary
CAS projection tests and semantic apply/observation tests passed. The provider
registry's domain method check also no longer imports the HTTP transport
package solely for the constant `GET`; exact read-only method validation is
unchanged and covered by negative tests. Other architecture failures, including
application-layer filesystem imports, remain unresolved and are not exempted.
The production Go run at `da77ff2fd` still reached the 20-minute runtimeapp
package deadline. Its active stack was in original attachment inventory
revalidation/directory traversal. The earlier local 6.812 s / 6.196 s commands
were exit-zero observations, not verified subcase passes: exact JSON-event
checking exposed the configured cache volume's removable-APFS fixture skip.
Those timings must not be used as behavior or performance evidence. Required
isolated Linux/macOS restart jobs now execute each complete subcase in a fresh
process under the same CPU/memory bounds and existing 20-minute Go deadline,
and report function-only CPU profiles. Exact subtest run/pass and package pass
events are required; zero selections, skips, failures and incomplete output fail
the gate. Full Go coverage and each process's deadline are unchanged. The cause of the long
Linux traversal still requires these measurements; do not infer deadlock or an
undersized budget merely from the suite timeout.
Neither these focused results nor the source baseline imply full regression,
native packaging or formal release readiness.

The subsequent CI at `322b722ee` executed both Linux held-state subcases:
evidence-registry passed in 219.78 s and pii-authorization in 384.30 s, including
the exact JSON-event gate. These prove isolated execution, not the full-suite
deadline or a product latency target. CPU profiles show substantial filesystem,
hashing and allocation work; they do not establish a deadlock. macOS failed
before the same behavior at package inspection. The diagnostic executable now
uses the physical `RUNNER_TEMP` path, preserving strict executable-path checks.
At `c9d946821`, the native sandbox raw-path assertions still failed while the
same read/write/link/child-process/ordinary-work assertions with canonical roots
did not. Production terminal, process and MCP composition already supplies
canonical protected roots. Both process test packages now use isolated canonical
temporary/home/configuration roots, with a regression for existing and absent
protected descendants through an aliased ancestor. Full local processsandbox
and process packages passed; remote confirmation remains required.
The package-inspection failure persisted with a canonical `-o` destination:
Go 1.26.4 executes its temporary build target, not that copied output. An aliased
`GOTMPDIR` reproduced the exact failure locally; physical `GOTMPDIR` passed the
new current-executable inspection test. CI now sets the actual build root and
executes this precondition alongside the held-state test.

That Application run reported 6,161 passes and four failures. Two actual
`/bin/zsh` parent-owned test contracts now join the required macOS integration
lane with unchanged deadlines. Missing cache configuration in the Milestone B
dry run is explicitly BLOCKED, still non-passing and nonzero when gated;
an invalid configured relative path remains FAIL. The oversized-source test
uses exact `Buffer.equals` instead of object-property traversal, retaining
byte/length equality and all no-side-effect checks. The focused 13-case check
passed; parallel local testing also exposed six settings timeouts, while its
isolated full 69-case run passed. Do not hide that resource/isolation evidence
behind a timeout increase.

Attachment creation-residue recovery fixtures now freeze the same original
inventory as production startup before entering recovery. The original failure
was reproduced; all six owner/leaf/shard preservation and independent-write
subcases then executed and passed. Production recovery validation is unchanged.
Registry architecture guards now distinguish the sole settlement producer from
its exact input/error-preserving shared-owner forwarding method, enforce the
current-owner lock lifetime, and bind import activation and finalization to that
same owner. Mutation checks reject changed arguments, owner selection, errors
and lock release. These focused guard passes do not waive the other architecture
failures; the local activation integration remains unverified because its
non-removable-APFS prerequisite causes a skip, which the exact-event checker
correctly rejects.

At `c9d946821`, Application CI subsequently passed **6,163 tests** (18 existing
or native-lane exclusions), and both Linux held-state contracts passed again.
The accumulated runtimeapp package deadline is now handled by four dynamically
discovered partitions per Go build mode. Every discovered Test/Example/Fuzz name
belongs to exactly one partition; no manual name list can omit new tests. All
other Go packages remain in the complementary package job. Every partition is
required by Development gate, retains the 20-minute process deadline, and
propagates nonzero exits/signals. The existing separate exact-event held-state
gate still rejects skips. Unit tests prove disjoint/full partitions and failure
propagation; actual local inventories were partitioned for both build modes.
This is coverage-preserving orchestration, not a full runtime PASS: remaining
implementation/fixture/architecture failures must still be resolved.

### PR #22 follow-up: persistence and CI attribution

CI run [34687727224](https://github.com/Eysn0130/analytix/actions/runs/34687727224)
at `b67058578` passed Application, Backend, all four Rust components,
Linux/macOS filesystem contracts, and **all four exact held-state restart
jobs**. macOS evidence-registry and pii-authorization executed in 310.95 s and
572.10 s respectively, with the no-skip JSON-event checker. The physical
`GOTMPDIR` correction is therefore remotely verified. That run still failed:
Source baseline found ShellCheck's masked-assignment rule, and macOS process
integration exceeded the Unix socket pathname limit under the long default
temporary parent. Separate assignment/export and a short physical `TMPDIR`
correct those fixture/orchestration causes; remote revalidation is required.

Four runtime partitions were insufficient: after 20 minutes ordinary shards
0/1 were at top-level entries 45/92 and 37/91, with the active tests only 20 s
and 9 s old. This is cumulative load, not proof those tests deadlocked. The
candidate uses sixteen complete dynamic partitions per build mode, at most
four independent runners concurrently, retaining `-p 1`, `-parallel 2` and
the original 20-minute Go deadline. Verbose execution exposes individual
durations. Partition completeness/disjointness, matrix alignment, invalid
indices and failure propagation pass five local tests. Both actual macOS
inventories partition completely (367 ordinary / 376 production names);
Linux selection remains platform-native. This is not yet a runtime PASS.

The history mutation failure was a real concurrency defect: benign terminal
appends may coexist with usage-index rebuilding, but compaction and rewind
must not rewrite history across that barrier. Commit `91ea62157` rejects
destructive rewrites under the same owner lock before effects. Original
history/event equality assertions remain; rewind rejection and a successful
post-rebuild retry are covered. Focused ordinary tests passed; production
compaction/rewind/terminal regression passed (69.803 s), the complete usage
index package passed (21.123 s), and the combined race check passed (28.666 s).
No product data directory or real Provider was used.

Commit `f813f41c9` makes the two CAS contenders take their preflight snapshots
before release, so both actually compete at the intended CAS boundary. All
winner/loser/disposition/restart assertions remain. Five ordinary and three
production repetitions passed; sequential competing admission remains denied.
Diagnostic tests now inspect the retained private error cause and assert the
exact closed public error separately; no public diagnostic exposure is added.

Provider-authentication regression was reproduced with an empty Registry:
legacy configuration does not authorize execution under the accepted
`local-provider-credential-authority` contract. Reusing the explicit synthetic
Registry Connect/readback fixture restores the actual loopback Provider call
and preserves the original structured-error/redaction assertions. The fixture
uses its isolated file-backed encryption authority, not the Owner Keychain.
Authentication plus existing lightweight-prompt integration passed locally
(116.890 s). Do not automatically grant credentials to every test: negative,
multi-provider and restart fixtures require their own explicit state contracts.

The deferred-registry activation and body-free report recovery guards now bind
the actual prepared closure/visitor, with mutation tests rejecting substituted
owners and readers. The complete architecture package at `839ead4d7` plus the
Provider fixture worktree still reports **11 failures**. Actual application
filesystem/ownership boundaries and stale lexical guards must be resolved
individually. Root/server/migration failures remain, and PR #22 is not mergeable.
Native installer and isolated packaged acceptance follow a genuine Development
gate; they are not implied by these focused results.

At `7d3e60a0f`, both Linux exact held-state jobs passed again (218.41 s /
282.41 s) after removing the unnecessary restart-lane `TMPDIR` override.
Canonical `GOTMPDIR` remains, and the short `TMPDIR` applies only to the native
socket lane. This establishes the bounded environment correction, not a claim
that all storage locations have equal performance. Source baseline and both
filesystem lanes passed. Full runtime/architecture regression is still open.

The macOS native processsandbox/process packages now pass remotely. The two
parent-owned Milestone A tests initially failed because their stderr digest
exactly matched the Owner cache backing-volume rejection: a synthetic parser
fixture still sourced the real hardware-specific preflight. Commit `a638a2734`
supplies a test-only sourced preflight bound to the parent's disposable
home/temp/cache, including exact exit-1 negative tests. Production preflight
and acceptance classes are unchanged. The complete local file passed 176 tests;
all four native Vitest cases passed remotely, but that job remained failing
on an unhandled AppShell dynamic import during teardown. A tag-excluded suite
does not run its `afterAll`; collection-owned preloads must settle before its
environment disappears. The test now explicitly awaits its own preloads during
collection, with deterministic pending/ready route fixtures instead of a race
against module-cache speed. Original loading/layout/boot assertions remain and
a ready-state check was added. All four tests passed locally; tag-excluded
collection completed without unhandled imports (not a behavior PASS). No errors
are caught or suppressed. The complete local native-tag lane then passed all
four selected tests (213.39 s), including the real D-0242 contract, with no
unhandled errors. Other cases remain covered by the complementary application
lane. This lifecycle correction still needs current-head remote verification;
four passing cases alone never
justify ignoring an unhandled error.

Commit `ab5a5ec08` also passed the three original tool-scope, invalid-model and
stale-thread-model tests (129.463 s) after explicit synthetic Registry setup.
Invalid models still cause zero Provider requests and the exact structured
failure. Existing capability/security assertions were not relaxed.

One public historical secret-scanning alert identifies a key-shaped negative
test fixture. The current fixture now constructs an explicitly synthetic
canary and retains its non-disclosure assertion. That does **not** prove the
previous literal was never issued, remove its public historical blob or justify
dismissing the alert. Resolve original credential provenance/rotation separately
without printing or exercising the value. No real Provider check is part of
these deterministic development tests.

PR #22 final-closure work began from `82e055026` with a clean canonical
worktree and matching remote PR HEAD; main was `58b365b3b`. Development run
`34690548012` passed Source baseline, Application, macOS process integration,
Filesystem and Backend lanes, but still had Go/Runtime failures. Those partial
results do not admit a merge.

The private-CAS constructor guard incorrectly treated a slice of existing CAS
handles as constructing authority. It now checks AST construction, including
negative cases for direct, pointer and nested implicit container literals.
The original-residue opening method was consolidated unchanged into the sole
CAS constructor owner, retaining access, binding, inventory and generation
checks. Local `go test -count=1 ./internal/architecture -run
'TestPrivateCAS|TestEveryRuntimePrivateCAS'` passed (1.585 s); the complete
`internal/adapters/outbound/finalauthority` package passed (183.916 s).
This closes that focused guard/ownership failure only; full architecture,
current-head remote CI and merge acceptance remain open.

Checkpoint restart preservation now delegates host path normalization and
physical-root alias overlap to its existing filesystem observer port. The
application retains the held-thread and recovery-source decisions. Symmetric
exact/parent/child overlap, adjacent-name exclusion and physical aliases have
focused coverage; complete local checkpoint and filestore packages passed
(0.469 s / 28.955 s). No overlap rejection, recovery budget or architecture
import constraint was relaxed.

Provider legacy-migration physical source reads now belong to the filesystem
adapter behind a read-only port. The Manager still owns one-use challenges,
source/owner equality and registry generation; absent source-reader authority
fails closed. The adapter retains bounded reads, physical identity before open,
single-link regular-file checks and before/open/after identity checks. Local
migration Manager checks passed (3.597 s); new adapter physical-boundary cases
passed (3.739 s), and the full adapter package passed (505.542 s). A concurrent
full Manager run timed out after 10 minutes in filesystem crash-point recovery;
process sampling showed sync I/O, so this is not recorded as full-package PASS.
No timeout or required CI coverage was changed. Current-head remote closure
remains required.

Funds CSV admission now receives its diagnostic writer from composition; the
application no longer accesses global process stderr. Its error identity and
stage/class diagnostic regression, and complete local admission package, passed
(0.641 s). Existing accepted-final observation/turn equality is now an error-only
domain validator, preserving the strict primary reader as the observation
producer. Complete local domain/evidence and app/evidence packages passed
(1.465 s / 48.723 s), including existing negative bindings.

Server active-history and child observation now use the immutable primary reader
bound by runtime composition, instead of constructing finalauthority in server.
The restart fixture binds the same real reader on reopen. Local active-history
regressions exposed that missing fixture binding; after correction, all four
compaction commit/crash-cut/absence cases passed (40.40 s), and stored-child seed
error preservation passed (7.83 s). Historical child lookup exposes a read-only
resolver using the original terminal validation and cloning, with no mutable
projection API. The marker-stripped source fixture now requires derivation
rejection, unchanged source/lineage/inventory and all original privacy checks;
its focused server test passed (1.35 s). These are local contract/recovery checks,
not a full server or runtime-suite acceptance.

Provider endpoint protocol-shape classification is now a pure model value
function shared by application and transport. This removes the loop's reverse
dependency on the legacy Provider adapter while preserving request URL parsing
semantics. Complete local loop/model/compat packages passed (20.416 s / 0.580 s /
0.394 s). Full architecture was still failing on the separately tracked media
HTTP composition and private report-owner references at this point.

Media execution now receives a transport factory through a narrow port. HTTP
client/request construction, committed proxy handling, bounded timeouts and
redirect refusal belong to the outbound adapter. Application-owned Registry
revalidation remains before send, after headers and after bounded body reads;
credentials are cleared and image downloads never receive the Provider header.
All existing loopback media tests and the new adapter transport tests passed as
complete local packages (0.490 s / 0.399 s). No live Provider was contacted.

The complete local architecture package now passes (12.008 s). Real violations
were fixed in their owners: private controlled-access outcome parsing/matching,
report settlement/history interpretation and restricted dataset-context matching
stay inside their application owners; runtime composes only bounded read-only
interfaces. Full local reportpublication/datasetsnapshot/piiauthorization
packages passed (5.943 s / 0.748 s / 0.842 s), retaining existing negatives and
adding invalid/missing/duplicate outcome-inventory coverage.

Stale guard assumptions were replaced by narrower structural checks, not
exemptions: original inventory observation requires its read-only prepared
physical bracket; copied-stage observation requires frozen roots and complete
Close/error propagation; historical report composition cannot gain current
capabilities or escape semantic validation. Mutation negatives exercise those
violations. Go test fixtures are excluded from the production dependency graph
using Go's exact _test.go rule; production files and alias imports still fail
negative tests. The old server line/file cap already failed at public baseline
`eed7dfb1a` (56 files / 10037 lines against 54 / 8649); it is replaced by exact
adapter-construction ownership, including alias/indirect/new/literal and duplicate
construction negatives. Other server layering guards remain. This is a local
architecture PASS only; current-head remote Development acceptance is pending.

Case turn snapshot admission now checks the composition-owned Registry's local
availability before contacting shared witness authority. The confirmed-import
owner retains its underlying snapshot capability and remains the only activator.
Restart fixtures explicitly seed synthetic protected Provider Registry authority
for ordinary loopback turns. Six focused zero-generation/partial/non-JSON/corrupt/
bound-sibling witnessed cases passed locally (151.68 s), retaining zero-witness
and preservation assertions. Fresh import activation passed (6.76 s); real CAS
readback-fault and exact-operation suites passed (13.57 s / 41.47 s). Combined
focused run passed (214.246 s), no skips. An earlier entire witnessed group ran
into its unchanged local 10-minute budget after reaching the last subcase;
that run is partial, not full PASS. Sampling and sequential progress indicate
cumulative filesystem cost; no timeout or CI partition was increased.

The two server child-delegation contracts used a stale synthetic source
capability lacking the prepared Registry effect surface. Their fixture now
binds one real signed legacy-store commit to its callback, exact source/context/
selection/prepared/marker and signed readback. No-op, repeat, mismatch, cancellation,
revocation and escaped-lease negatives remain fail-closed; this is synthetic
legacy public-contract evidence, not V2 shared-witness acceptance. A concurrent
local run subsequently missed parent continuation near its existing task budget;
no timeout or assertion was changed. Stable serial ordinary and DeepSeek public
contracts both passed (41.73 s / 43.09 s), including parent=3/child=1, typed handoff,
durable restart and all original raw/private non-disclosure checks. Primary
reader missing/rebind/cancellation coverage passed (1.73 s); combined local
server selection passed (87.203 s). Full current-head remote server/package
acceptance remains required.

Thread listing now excludes only explicitly qualified restart-held scopes before
public snapshot/cursor reads; independent read failures still propagate. The
complete local thread application package passed (2.212 s), and the full varied
inventory ordinary HTTP/witness-outage test passed across two restarts (319.88 s).
Signed report-attempt fixtures without original primary/enrollment now assert
their actual earlier authority rejection instead of expecting a later recovery
gate. Reserved and corrected aborted cases passed (24.86 s / 26.97 s); all three
Core-linkage negatives passed (66.53 s), and the genuinely enrolled reserved
history retention test passed (0.48 s). The combined initial selection failed on
the old aborted assertion; its corrected focused rerun passed. These are local
focused results, not full runtime acceptance. Remote run 34693285203 independently
reported a plan-turn 500 in the Ubuntu native PII held-state lane; investigation
continues before merge admission.

The CLI private-frame HTTP contract now distinguishes a cold Registry from a
real committed historical Registry. The latter is provisioned through signed
prepared history, actual Registry CAS and the enrolled shared witness, with
matching durable turn/grant/result/epoch and case-thread authority. Cold startup
and admission remain local; committed startup performs read-only reconciliation,
and an outage on the subsequent case HTTP request reaches the exact shared
namespace without advancing it. Both retain typed zero-fact source-unavailable
output and the private-frame/ambient/ready non-disclosure checks. The subprocess
uses one isolated filesystem and its actual TMPDIR for authority and temporary
durable owners; compiler caches remain external. The existing 30 s ready budget
is unchanged. Final local focused execution passed both scenarios without skip
(cold 3.33 s, committed 5.17 s; package 9.194 s). This is synthetic CLI contract
coverage, not native dataset, Provider, packaging or full remote acceptance.

Remaining runtime contract fixtures now reflect their actual producer seams:
CSV materialization fields match the native producer's flattened wire contract;
late-failure startup cases keep the earlier child identity inventory valid and
inject an invalid lifecycle in the later migration phase; inherited legacy final
display is taken from the real source finalizer before testing sealed read-only
preservation. Publication negatives assert the current exact original bytes,
mode and absence rejection, preserving all mutation and independent-write checks.
Local individual results passed: CSV composition 1.93 s, pending-work late failure
4.19 s, final-event late failure 7.95 s, seven publication mutations 6.11 s,
inherited final readers 7.74 s, and signed-added-dependency rejection 4.21 s.
The two combined selections also included an unfinished CASE fixture and failed
on that separate case; they are not full runtime PASS. The intermittent async
second-restart equality test passed locally (45.56 s) with bounded metadata-only
failure diagnostics; its remote failure remains unresolved until current-head
CI establishes the cause and result.

The intermittent plan failure has a deterministic privacy-projection defect:
a valid RFC3339Nano savedAt with seconds 13–19 and nine fractional digits is
classified by the general prose scanner as an eleven-digit phone number. Seven
fixed inputs failed against the original event projection despite valid typed
PlanStatus. The existing exact host-bound, canonical closed-plan metadata path
now preserves validated savedAt alongside its SHA-256 contentHash. General text,
lookalikes, invalid timestamps and phone-bearing display fields retain their
original privacy rejection. All seven regressions and negatives passed; the
whole local event domain package passed (0.341 s). Remote failure timestamps
fit this mechanism, but current-head runtime and native PII CI are still needed
to establish remote closure. No timeout was increased and no raw-error diagnostic
was added to HTTP, persistence, model context or production logging.

The CASE terminal-closure fixture now starts with a real nonempty witnessed
Registry and its exact durable originating turn, signed preparation, grant,
result marker, epoch and case-thread authority. Counts distinguish that original
history from the single new terminal chain and prove the historical records are
unchanged. The complete local candidate/fixed/longitudinal test passed (381.97 s):
each actual failure phase drains, recovery authenticates the original winner,
and same-thread ordinary continuation preserves original CAS/private bytes.
This result covers an already committed Registry; first-import/restart continuity
with an empty Registry remains a separate contract under review. The combined
selection still failed on desktop TypeScript retirement before its later owner
migration, so no full-runtime or desktop-migration PASS is claimed.

Checkpoint preflight no longer interprets a failed Git subprocess as a clean
index. The filesystem/process adapter proves a non-Git workspace by a complete
ancestor inventory; repository probes retain containment and propagate a fixed
unavailable result on lookup, configuration, containment or execution failure.
The consumer blocks mutation on unavailable or missing probes. Local focused
process checks passed (1.098 s), including actual contained staged detection and
protected configuration denial; the unsupported-containment case belongs to its
non-macOS build and awaits Linux CI. Filestore mutation/UTF16 checks passed
(0.690 s). The macOS HTTP staged checkpoint contract passed (50.57 s), with target
and Git index byte equality. No unrestricted Git fallback or safety waiver was
introduced; Linux protected repository probes remain explicitly fail-closed.

G5 Go conformance now uses the actual product display name Analytix and the
current useSpeechToTextEnabled reader already enforced by the TypeScript source
scanner. Runtime/package machine identity remains analytix. The facade, source
implementation and evidence requirements are retained; four new negative
mutations reject direct facade, missing source, retired reader and absent
source evidence. Full G5 control executable comparison and those negatives
passed locally (0.662 s). The preceding root selection exposed the stale reader
name after its other four current Registry/Git contracts passed; that combined
selection was FAIL, not full application or Go acceptance.

The committed Provider resolver now retains the desktop's key-free tariffs,
reasoning protocol and aliases only for the exact Registry-admitted route and
canonical models. The Registry remains the sole credential, selection, endpoint
and executable model authority; legacy profile capabilities are not copied.
Ambiguous aliases or duplicate attributed routes fail closed. Deterministic
metadata regressions failed before the fix; the production-mode resolver/pricing
selection passed afterward (16.280 s, no skip). Public HTTP regressions passed
for four-provider pricing (81.88 s), cache accounting (58.07 s), caller Provider
IDs unable to override selection (40.93 s), exact-attempt DeepSeek reasoning
(57.16 s), subagent model/effort/tool scope (52.92 s), selected Xiaomi (64.23 s)
and its canonical alias (64.12 s). These fixtures perform actual protected
synthetic onboarding and explicit selection through the Registry HTTP contract;
plain unavailable-authority constructors remain unchanged. Full root-package
and current-head remote acceptance have not yet passed.

First-import continuity is a separate, precisely bounded next-stage seam.
Current confirmed admission persists DSV2 and activates an in-memory Registry
service without requiring an evidence receipt. Before the first receipt,
restart intentionally leaves the generation-zero Registry dormant. The normal
stage/confirm entrypoint can admit a new source revision and reactivate it; that
is not automatic restoration of the prior activation. Code review found no
accepted receipt-free activation continuity requirement that authorizes
removing the current fail-closed gate. No first-import/no-receipt/restart
acceptance is claimed here, and the committed-Registry closure fixture above
must not substitute for it in isolated-development or packaging acceptance.

Runtime shard admission now validates streamed Go JSON execution, in addition
to deterministic inventory partitioning: every selected test and observed child
must run and pass exactly once. Empty, missing, duplicate, foreign-package,
skipped, failed and incomplete execution is rejected. The five synthetic
ordinary lifecycle faults move to a required macOS lane with a current-source
schedule entrypoint; that lane also executes the contained schedule contract
in both build modes. Its ordinary-mode root selection covers ten exact stdio,
Git and Bash contracts, independently of the Linux package inventory. The
successful-stdio contract is compiled for Darwin, where containment is supported;
the unsupported-host process rejection and Linux Bash behavior remain covered.
All receiver names must exist in the actual discovered inventory. The same
20-minute Go budget and sixteen runtime shards remain in force.

Four pre-existing explicit external diagnostic entrypoints are classified before
execution and logged as not_executed, never as required PASS or allowed skips:
TestRuntimeOptionalPublicFixturePathV1, TestRuntimeOptionalPluginPublicConsumerV1,
TestRuntimeOptionalPluginUnauthorizedFirstMCPTerminalDiagnosticV1, and
TestLocalNonPublishablePackageInspectionWhenExplicitlyProvided. They require
Owner task/corpus/diagnostic authority or an exact packaged executable/data.
Missing schedule build inputs are not part of this exclusion. The new platform
job belongs to the same required Development gate. Root-reviewed Node partition,
receiver and gate regressions passed (8/8, no skip); real platform and complete
remote execution are still pending. CI admission is stronger, not relaxed.

Root HTTP tests that actually execute a synthetic Provider now explicitly
onboard it through real Registry revision/incarnation CAS and protected secret
ingress. The opt-in fixture rejects non-loopback endpoints and never repopulates
an existing Registry on restart. Missing-authority tests retain their plain
constructors. Provider selection tests perform the real select operation;
caller/profile Provider IDs cannot create a second execution authority.

Live turn, multi-model and goal-lineage tests now create real threads rather
than reuse a G2 static completed turn lacking current terminal credentials.
The five-family replay/cache/atomic-terminal test passed (107.84 s). Goal lineage
is produced by the actual terminal-backed goal completion path, not a fabricated
event; its exact goal/thread/turn binding and private projection passed (48.85 s).
The turn-field contract passed (39.29 s), and real contained stdio startup/catalog
plus mutation-authority rejection passed on macOS (37.89 s). Subagent execution
keeps the admitted model/variant/source fingerprint but must not persist Registry
endpoint authority; the positive HTTP/route/absence test passed (47.68 s), and the
unselected-Provider zero-call negative passed (49.35 s). Earlier combined runs
failed on stale route-field assertions and fixture cardinality; they are not
full package PASS. Complete new-head remote root coverage is still required.

Desktop retirement now recognizes exact-witnessed legacy TypeScript parents
whose original product storage contains only metadata/messages sidecars. The
ordinary primary inventory remains strict. Only the desktop retirement path
carries that bounded audit input into the semantic stage, where the normal
recovery/upsert path materializes a canonical primary before strict inventory
and signed publication. Invalid or missing lineage is checked before initial
authority directory creation and repeated during semantic admission. A new
negative initially exposed two housekeeping directories created before rejection;
the earlier witness fixes that mutation instead of weakening whole-tree equality.
The final complete desktop-migration selection, strict/wrong-family/unwitnessed
inventory checks and both invalid-lineage no-mutation branches passed locally.
Current-head remote runtime acceptance remains required.

The first stronger-admission remote run at 7099f0015 exposed two additional
issues in the newly required native lane. The schedule HTTP fixture still had
legacy configuration without admitted Provider Registry authority; it now uses
the same real synthetic onboarding fixture before composition. Its production
focused contract passed locally (7.47 s), with current-source Node entrypoint.
The workflow command substitution now separates assignment from export so
actionlint can observe failures (SC2155); pinned actionlint 1.7.12 passed.
Remote Application, production non-runtimeapp package tests (including
architecture and server), and completed native contracts passed at 7099f0015.
That SHA is not a complete required-CI PASS: source lint and both runtime-platform
lanes failed. Both platform runs exposed a one-shot inherited replay timeout
after fork/restart in the missing-optional-plugin scenario; the other four faults
passed remotely. This timeout remains under investigation without a budget
increase. A local external-filesystem run separately exhausted the unchanged
20-minute cumulative package budget after three complete faults; this is not
a full lifecycle PASS. Synthetic runtime profiles may use isolated native temp
roots while compiler/dependency storage remains on the configured cache volume.

At 7099f0015 the complete ordinary root inventory executed all 321 tests:
312 passed and nine failed, with zero unstarted tests. Eight failures were
remaining contract/fixture drift. Attachment and research positives referenced
an uncreated historical workspace; they now create real isolated workspaces
and threads. Attachment restart reuses the same durable root and owner identity.
These three public tests passed locally (3.91/3.74/8.46 s). The public-final
checks now require V3 acceptedFinalView, reject private acceptedFinal in public
records, and validate complete durable publication slots and payload digests.
Cross-parent continuation retains the closed validation_error classification
and all non-disclosure/zero-child-call negatives. Those four tests passed
locally (4.80/3.66/3.81/8.88 s).

Vision capability configuration is an intent/limit fixture, not executable
authority. Its legacy route cannot establish Registry media admission or the
still-unimplemented trusted local image privacy projector; runtime-info must
remain unavailable and withhold that route. The corrected negative passed
(3.27 s), retaining enabled intent, probe metadata, budgets and secret/endpoint
redaction. No working image/vision path is claimed or implemented in this CI
closure. The remaining stale-child startup fixture and native lifecycle cost
are still being resolved; these focused results are not full remote PASS.

All ten exact native root process contracts passed locally (43.086 s, no skip).
The missing-optional-plugin lifecycle completed locally in 347.894 s with CPU
profiling. Its profile attributes 209.33 s cumulative CPU to pending trusted
inventory and 193.60 s to individual secure receipt reads. Snapshot inventory
already reads all receipts twice; disposition validation redundantly rereads
each receipt and rescans the protected CAS tree. The pending-store fix is being
validated at that owner boundary, without caching across requests, changing
replay authority checks, or increasing the existing timeout.

The ninth ordinary-root failure was an unsigned orphan child fixture. Current
Core startup intentionally rejects that inventory before recovery; treating it
as an accepted stale child would bypass signed producer association. The same
fixture now exercises the error-returning constructor and proves exact
ErrChildProducerInventoryIncomplete, unchanged preserved root contents/modes
and zero Provider requests (0.44 s). It does not manufacture a receipt or reopen
a completed job. The existing legitimate producer-allocation, bound parent
recovery, and stale/lease-expired/orphan recovery endpoint tests were separately
executed and passed (server selection 1.980 s). This is constructor-negative
and focused recovery evidence, not a new crash-process acceptance claim.

Pending inventory now reuses only the already-validated receipt set from the
same observation. Both real CAS listings, both ordered observations, root/store
locks, exact disposition-to-receipt binding and changed-inventory rejection
remain intact. Ordinary ListDispositions still performs its original reads.
The new access-count regression failed before the change (four receipt CAS
accesses at one disposition versus two at zero) and passes afterward with a
constant count through four dispositions. Missing, mismatched and duplicate
receipts, deletion between observations and existing concurrent append tests
pass; the complete pendingworkstore package passed (10.192 s).

Under the same isolated native-temp profiling setup, the complete missing-fault
lifecycle fell from 347.894 s to 141.900 s, with every assertion retained. The
full production platform partition then passed (391.129 s): current-source
schedule contract plus all five lifecycle faults, admitted by the exact JSON
execution checker with no skipped or missing test. Broader local pendingwork,
thread, casethread and architecture packages passed. The whole server package
run failed a pre-existing blanket /private/ substring assertion when its
ordinary fixture workspace used /private/tmp; that is being checked separately
and is not reported as a full server PASS.

Remote ordinary runtime shard 8 at 7099f0015 exhausted the unchanged 20-minute
package budget after successful tests; its active cancel_after leaf had run
only one second. The largest completed test was the joint held-owner ordinary
HTTP test (639.80 s), plus 144.30/140.73/119.05 s tests in the same partition.
The held fixture has only one pending receipt/disposition, so the lifecycle
optimization cannot be assumed to solve this separate cumulative cost. A
focused held/pii profile is in progress before any further orchestration change.

The local server privacy failure was a boundary mismatch in the assertion:
LoadEventsSince returns internal events including the legitimate ordinary
workspaceRealPath. A blanket /private/ match therefore classified the isolated
/private/tmp workspace as an error leak. Durable replay still rejects the exact
fault canary and full persistence root. Public privacy assertions now also run
the actual production projector, retain the /private/ restriction, reject
workspace/securityContext disclosure, and require a visible turn_started event
so withholding everything cannot satisfy the check. All nine original
success/failure/cancel by archive/events/usage cuts passed (package 3.008 s).

The held/pii profile completed successfully (369.11 s test, 368.25 s profile
wall time). It attributed 274.78 s cumulative CPU to original-create residue
revalidation, including repeated protected filesystem observations. Those
currentness checks cannot be removed without a separately proven owner
contract. This is distinct from the corrected pending receipt scan.

All eight tests cut off or not started in the remote ordinary shard 8 run
passed in a fresh local exact-selection run (52.737 s, no skip or missing test).
The create-residue recovery test took 7.09 s. These are focused local results,
not a completed remote shard. Existing remote timing samples cover 360 of 368
required top-level tests; the remaining eight now have local evidence, which
cannot predict identical hosted-runner timing.

The CI partition correction reserves shard 15 for the exact held-owner test
and distributes all other automatically discovered required tests across the
remaining 15 shards. Both build modes, all 32 required runtime jobs, concurrency
and the 20-minute package budget remain unchanged. Missing dedicated identity,
empty selection, skipped/incomplete execution and child-process failure still
fail closed. Eight orchestration regressions passed, including full inventory
coverage, disjointness, deterministic ordering and minimum inventory boundaries.
Actual current-SHA remote execution remains required; this scheduling correction
does not establish a product startup-latency improvement.

After the public-projector assertion correction, the complete local server
package passed on the stable source candidate (139.422 s, ordinary mode,
-count=1 -p 1 -parallel 2 -timeout 20m, isolated native synthetic temp profile).
This closes the nine local closure-privacy failures without claiming a remote
or production-mode rerun from this local result.

## PR 22 legacy-source review delta (2026-09-13)

Review-conversation inspection found eight CodeQL path-flow annotations
(129–136) in the legacy Provider source adapter. They share one request path
flow. Application authority consumes an exact one-use challenge bound to the
retained recovery before the adapter runs; the adapter checks device/inode,
single-link identity and the canonical physical-path digest, and the application
then checks source bytes/hash and the complete owner challenge contract.
Those custom constraints must be considered when interpreting the static flow;
no scanner exclusion or broad sanitizer is added.

The review also identified a concrete open/read race: the old helper opened a
path normally and checked the opened object's identity only after reading.
Replacing either source or owner with a FIFO after observation blocked the
read. A bounded subprocess regression reproduced both failures before the fix
(5-second deadline each, child reaped). This is a direct PR safety issue and is
being corrected at the filesystem adapter, without changing migration authority.

The race fix now uses a no-follow, nonblocking Unix open and validates the opened
regular single-link object, expected identity and bounded nonempty size before
reading. Windows uses a reparse-point handle and disk/type/link checks without
an ordinary-open fallback. Existing post-read identity checks and safe errors
remain. Complete providerregistryfs passed (98.442 s), focused migration and
recovery contracts passed (136.515 s), and the integration owner's focused
physical-boundary/replacement rerun passed (7.096 s). Windows amd64 compiled
successfully; no Windows runtime or packaging acceptance is claimed.

Remote run 34704036542 at 29cacee53 had passed Source baseline, Application,
Backend, both filesystem platforms, macOS process integration and both Linux
held-state owner lanes while other required jobs were still running. These
results precede the legacy-open fix and do not establish its exact-head gate.
The next pushed SHA must receive complete required CI before Ready or merge.

At 9017375e7 the remaining CodeQL path-flow alerts were 129–134 and 137–138
(the moved open/read locations received new alert identities). Each was
reviewed against that exact candidate and classified as a false positive for
the custom recovery/challenge/physical-path/source-owner constraints described
above. The disposition links to PR 22 discussion_r3996804873; the eight review
conversations were resolved. CodeQL reported SUCCESS after this manual triage.
This is an evidence-backed alert classification, not an unqualified automated
security proof, a disabled query or elimination of unrelated CodeQL debt.
The independently reproduced FIFO defect was fixed and tested before triage.
Complete required CI and post-merge CI remain the authoritative merge evidence.

<a id="snapshot-d469-consolidation-97"></a>

### consolidation: original lines 97–166

Source: `docs/analytix/document-consolidation-register.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `1a2b336672401b20cb596be06295fe2c36ee2965a6df75c82f126bdae9a17440`.

### 2026-09-30 有界资源与 quarantine 治理

基于 `fb210a84426f8bcce55e515c681422702f54232c` 的 canonical worktree；
四项继承 dirty 不属于本切片，哈希保持不变。此记录是局部源码验收，
不是系统治理、安装验收、同模型分析增益或 main/发布就绪结论。

- `runtimeapp/app.go` 的七处资源绑定失败出口收敛为一次有序绑定和一个出口；
  owner 仍在 `owned_runtime_resource.go`，原可选条件、defer、错误链与最终
  接管时点保留。反向顺序 fixture 得到行为 RED；多资源 drain/close 重试、
  已关闭前缀、未关闭后缀、必需 nil 与并发 shutdown 的定向 race 检查通过，
  真实 handler/maintenance 定向组装检查通过。
- quarantine 的唯一行为 owner 为 `backend-manager.test.ts`：synthetic overrides、
  冷导入、默认 GET/POST、并发 ensure、合法 renderer lease 与 stop 路径，
  覆盖进程、目录、监听、网络和清理写入。五项临时语义回归均得到行为 RED，
  原生产 blob 恢复哈希相同后才退役旧八项禁词及 packaging 的重复簇。
  规范化 module identity 检查保留执行 authority 的架构边界。
  与 native-runtime-paths/packaging 的聚合报告实际发现并通过 79/79，
  其他授权、泄露、平台与聚合门禁不变。
- 根指南初次只减 363 字节；在 `62aa16969` 冻结基线继续收敛自动适用链，
  root 15,467 → 13,945，src/go/docs 分别 -565/-73/-581。重复语义依赖
  已自动读取的 root，不新增全 imports，独立仓库安全守卫保留。
  global 9,094 不动，常驻 global+root 24,561 → 23,039；scope 按路径追加，
  不能把本次跨三 scope 的指南总量当作日常常驻。
- 旧三文件所选文本脚本计数 4,602 → 7,987：app -556、helper +557、test
  +3,384。app 缺少完整初始化/defer/接管上下文，旧 receipt 未绑定完整
  返回 payload；不能称为完整需求读取或 Agent 摄入。replay 各一调用，
  排除初始化/定位/实现/失败/轮询；这些成本并未消失，全任务总量未知。
  原 cache 失败、基线取消（130）及旧 null 保留。20.356s / 8.943s 的
  warm/编译缓存未受控，新增测试工作量不同，不推导速度、token 或费用收益。
- 本切片生产源码净 +1 字节，测试净 +7,086 字节；维护收益是失败处理
  修改点 7 → 1，不能以总 LOC/字节下降代替覆盖或收益。没有新增治理工具。

已启动的第二生产 owner 切片将 adapter 六条路由的 body/schema 注册合为一处，
summary 的 schema、identity 和精确投影选择由同一私有 resolver 负责；
严格 canonical equality、AcceptedFinal/currentMain pin 与两边 sanitation 未迁移。
同一 `runtimeRequestViaHost` 测试组 before/after 各 43 通过、73 未选，
最终候选 44 通过（包含两项实际 HTTP 回归）；组内其余项目混合直接 sanitizer
与 HTTP 测试，不称为全组真实 transport 证据。精确投影相关三文件 43/43、
web/node typecheck 通过；独立只读 review 无具体缺陷。该切片生产 +601 字节、
测试 +2,477 字节，不宣称整个 adapter 或系统技术债已收口。

本机小 receipt 在 `/private/tmp/analytix-debt-resource-matched-{before,after}-20260930.json`
及 `analytix-quarantine-{red,green}-20260930.json`；前者是明示范围的读取 proxy，
后者保留五项 RED 与执行式发现结果，不复制大型 inventory。

整改固定 17 个完整必要文件（含新增 helper/测试、适用指南及既有 fixture），
清单和哈希在 `analytix-debt-cost-frozen-{before,after}-20260930.json`；物理
corpus 与实际完整指南返回 payload 分开计数，register 增量是 corpus 内的
治理成本分解，小 receipt 和验证证据另计。测试数据表减少 529 字节，
关闭/错误身份/重试/并发及冷导入、caught 副作用、两 packaged 态、lease
覆盖保留；反向关闭及五类副作用在新测试上再次 RED，生产 blob 恢复一致。

在 `aa76582cc3e66af5c5b3609d0bd9d9bee535ff1f` 上，packaged Milestone A
测试的八处 Git 仓库种子声明收敛为一处本地 fixture，各场景仍独立建库和清理；
完整 test owner 含新增 helper/import 从 567,614 降至 565,458 字节（-2,156），
生产 owner 的读取需求未减少。前后实际发现的 179 个 ID 相同，选定 8/8 通过，
18 处断言原文不变，171 项未执行；web/node typecheck 通过，独立只读复核无缺陷。
同进程快照缺失、消费后失败/replay、目标隔离、dirty/prepackaged、symlink 和
Git closure 边界保留。固定比较的 10 个完整文件基线为 823,568 字节；另读的
CI、validation-command、packaging-config 测试共 210,352 字节为定位/复核输入，
不能省略为全任务成本。小 receipt `analytix-milestone-source-fixture-{before,after}-20260930.json`
分别绑定文件哈希、register 增量和验证结果，不推导全套测试、token、速度或发布结论。

`790b1b140` 基线上的 raw 账户 ingress 修复恢复既有案件纵向索引；冷启动不造索引，
同一当前 lease、原生实体 descriptor 和受限 projection 保留。owner 回归先 RED、后
8 项通过；实际 HTTP 连续性及撤权请求回归通过。新增两线程 CNY 数值 fixture
复用原 Go/native/Final 链和独立整数 oracle，但原生执行仍 blocked：旧输入的 Host
信任校验失败，`aac3ccef` 包的 Darwin fuse 摘要也不符合当前契约。元数据恢复不含
金额/笔数复用；完整初始请求字节与请求级数值计数尚待原生执行，不宣称 SQL 次数、
token、智能或安装验收收益。小 receipt 为 `/private/tmp/analytix-longitudinal-verification-20260930.json`。

<a id="snapshot-d469-consolidation-170"></a>

### consolidation: original lines 170–177

Source: `docs/analytix/document-consolidation-register.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `59a4b4d9730867b0afef8ca497a94fbda60ca858b026ddfd58777db5acba83bc`.

当前交接与矩阵的 2026-09-27 叙述已迁入既有
[日期 QA 的 Historical appendix](qa/pr28-next-execution-2026-09-26.md#historical-entry-snapshots-relocated-from-c2cd--2026-09-29-pdt)，
保留旧 entry anchors。入口现在区分 c2cd HEAD、继承的未提交 host-local B1、
7dc 准确安装证据与当前未知远端状态。只读
`node scripts/validation-burden.mjs --json` 使用
[小基线 manifest](validation-burden-baseline.json) 比较声明读取字节并盘点静态测试范围，
接入既有 `verify:baseline`。静态入口重叠不等于 CI 重跑，未知依赖仍走原全量门禁；
报告不是低价值测试删除列表，也没有实际 token、正确率或首次有效验证时间基线。

<a id="snapshot-d469-consolidation-208"></a>

### consolidation: original lines 208–234

Source: `docs/analytix/document-consolidation-register.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `41dc6f971aba6fdc105ecf84867ba53479339a19ad563a38988737e1ace7cd6c`.

## 2026-09-09 本轮处置

本轮 [审阅记录](documentation-delivery-review-2026-09-09.md) 修正当前入口的
Hub-first 冲突、插件身份归属与规范分类。文档地图中的 2026-07-29 外部指南比较
迁至 [Historical reference](upstreams/agent-guidance-review-2026-07-29.md)，
原入口保留链接。历史报告、归档 change 和打包要求的 reference 镜像保留；
不会因为文件长、旧或重复就删除仍被消费或用于证据追溯的内容。

## 2026-09-21 PR28 current-entry consolidation

Scope: the two actual current entry files and their existing dated evidence owner,
not a whole-repository documentation approval. START d8640957c; product SOURCE2f25c22f7.

| Original at d8640957c | Exact original SHA256 | Preserved owner / action |
| --- | --- | --- |
| `handovers/README.md` | `57d367da2f4971651f1627f9ccdd71c674c83700716d2cf566b93379ea4714c2` | Complete snapshot moved to `handovers/2026-09-16-pr28-continuation.md`, explicit historical heading IDs; README now current identity/gaps/recovery with compatibility links |
| `product-completion.md` | `c2d7429c6fc7a389587535ef0a606e322bed94704ac7461b4339144491aa881e` | Complete snapshot in the same dated owner; product-relative links rebased, current file becomes P0–P5 as-built/gap/evidence/gate matrix |

Original text remains recoverable at the exact Git commit; migration changes heading
currentness labels and link locations, not recorded outcomes. Private migration
receipts bind original/relocated hashes and each heading's old/new anchor. The current
QA adds errata for the wrong LibreOffice anchor, browser-red executed-versus-discovered
counts, and old65ab totals. It does not rewrite immutable command receipts.
The document map and knowledge governance now preserve a newer unsynchronized local
writer/checkpoint before consulting an older remote. No license, historical failure,
compatibility reader or product feature was removed. Other documents are UNREVIEWED
unless a specific reading/validation record says otherwise.
