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
