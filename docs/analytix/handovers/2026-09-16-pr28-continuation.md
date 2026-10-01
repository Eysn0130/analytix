# Analytix PR #28 跨线程接续检查点

Status: Historical checkpoint；当前恢复入口为 [README.md](README.md)，下文日期/SHA仅是历史证据
Applies to: `codex/workbench-product-delivery-20260914` / Draft PR #28 的接管、暂停、恢复与继续施工
Current as of: 2026-09-16；实现候选观察点 `5a2db4e67c739b0fab41ed48cde6fe4d16f13de7`
Source of truth: 当前 Git/GitHub、当前代码与测试、适用 accepted specs / OpenSpec、当前精确 HEAD 的 fresh CI / review evidence
Supersedes for current routing: `2026-09-10-owner-replacement.md`；后者继续保留为历史检查点

本文件的目的只有一个：**换 ChatGPT / Codex / Owner 线程后，可以从仓库本身恢复正在做的事，而不依赖聊天记忆。**
任何 SHA、CI、PR、任务状态都必须在恢复时 fresh 核对；本文件只提供被核对的起点和依赖顺序。

## 1. 当前 GitHub 施工位置

| 项目 | 当前检查点 |
| --- | --- |
| Repository | `Eysn0130/analytix` |
| Default branch | `main` |
| `main` observed SHA | `ce96cf12581acfa0e19fae7c6aa9c709371012c8` |
| Active construction branch | `codex/workbench-product-delivery-20260914` |
| Implementation candidate observed HEAD | `5a2db4e67c739b0fab41ed48cde6fe4d16f13de7` |
| Pull request | Draft PR #28 — `Integrate workspace tabs and native document annotation changes` |
| PR state | Open / Draft；不要把 `mergeable` 单独解释为 ready-to-merge |
| Current authority | 当前用户请求允许继续 Analytix 施工；不等于自动授权 merge、tag、release 或绕过门禁 |

本次交接治理提交会使分支 HEAD 前移，但不会改变上表 `5a2db4e...` 所绑定的产品实现候选事实。
新线程必须重新读取分支 HEAD，并把“实现候选的旧证据”与“治理提交后的新 HEAD”分开报告。

## 2. PR #28 的已接受施工方向

当前范围以 PR、代码和 [`../qa/tabbed-document-annotation-2026-09-14.md`](../qa/tabbed-document-annotation-2026-09-14.md) 为准。恢复时不要重新发明产品方向。

当前已明确的范围包括：

- 保留主对话，不因工具/对象 Tab 切换而重建会话；
- 右侧工作区、文件/文档对象 Tabs 与底部终端形成统一工作区；
- Office 采用原生只读交互与标注，不新建一套手工 Office 编辑器；
- 选区/对象引用进入主输入区，由 Agent 读取受控 scope 后提出修改；
- 修改必须经过对象、版本、序列、插件 generation 等权威检查，并在确认后执行；
- 保存、撤销、重开和冲突路径必须保留真实回执和恢复语义；
- 不扩大宏执行、外部资源、路径写入、Provider、权限或 package admission；
- fixture、合成引擎、真实 GUI、真实 Provider、packaged 分发分别记账，不能互相冒充。

如果新的用户请求明确改变该目标，先记录 target 变化与影响面，再施工；不要默默把旧 PR 解释成新范围。

## 3. 已有证据，只按记录的候选和环境成立

截至当前 QA 记录，已有的 focused evidence 包括：

- 原生/工作区/打开操作测试 137/137；后续恢复与退出保护增量 44/44；
- IPC / public contract / file tree / summary / workspace helpers 76/76；
- NativeOfficePanel / native-reference-store 22/22；
- 既有 Write / 主线程回归 48/48；
- TypeScript typecheck 与记录范围内的构建/ESLint；
- XLSX / PPTX / DOCX 固定 WASM 合成路径的原生选区、受控修改、导出与重开实验；
- 后续 Go 边界修复的 focused gate / regression evidence。

这些结果的详细边界、测试 SHA 和不得扩大声明见 QA 文档。不要把较早 SHA 的完整构建、WASM 实验或 synthetic Provider 结果转记成当前 HEAD 的 GUI / package / live-provider PASS。

## 4. 当前未闭合项

### 4.1 精确候选 CI

Development CI run `35130866674` 在实现候选 `5a2db4e...` 上首次完成为 FAIL。
已定位的失败 job 是 `Rust unit contracts (analysis_compute)`：

- 该 job 的库测试 286 个通过；
- `stats_query_cli` 中 `rows_filter_cases::query_stats_rows_cli_filters_source_date_and_search_text` 失败；
- DuckDB 报 INTERNAL assertion：`Attempted to access index 0 within vector of size 0`；
- 原 job 结果不能改写为 PASS。

2026-09-16 已只对这个有判别力的失败 job 发起一次 rerun；本检查点写入时 rerun 正在执行 `Test native component source contracts`。
恢复线程必须先查看该 rerun 的最终结论：

- 若再次失败：按真实回归处理，先最小复现 / 缩小根因，再修复；不得通过 skip、弱化断言或降低 gate 来“变绿”。
- 若通过：可记录为“同一 SHA 单项 rerun PASS / 首轮存在 DuckDB internal failure”，但不能凭一次通过声称已完成根因修复；是否需要进一步稳定性工作取决于复现性与风险。
- 治理提交产生的新 HEAD 必须看自己的 current checks；旧 SHA 的 rerun 只能作为相关源码未变时的限定证据。

### 4.2 GitHub Advanced Security / review

PR #28 已有 GitHub Advanced Security review comments，包含 object-editing 文件路径边界相关 CodeQL 反馈。
当前工具不能直接枚举 code-scanning open-alert API，因此**不能在本检查点宣称所有 CodeQL alert 已关闭**。
恢复线程应读取 PR #28 当前 review conversations / CodeQL checks，并针对当前代码逐条确认：已修复、误报且有边界证据、或仍需施工。

### 4.3 GUI 连续旅程

当前应用 GUI 的“标注 → 主对话 → Diff → 应用并自动保存 → 撤销 / 重开”连续旅程仍是 BLOCKED / UNVERIFIED。
既有合成 QA 身份的 Keychain 被锁定，原控制入口不可恢复；此前没有新建身份、复制凭据、绕过解锁或触碰真实 Provider。
该外部环境阻塞不等于产品失败，也不得被写成 PASS。
独立 deterministic / focused 工作可以继续，不应因这个外部 seam 停止全部施工。

### 4.4 PR / release 状态

PR #28 仍是 Draft。本检查点不授权或宣称 merge、tag、package release、Product RC 或 Formal RC。

## 5. Active OpenSpec 与 PR #28 的关系

当前仓库仍有两个非 archive change：

- `openspec/changes/case-evidence-publication-gate/`
- `openspec/changes/define-agent-platform-brand-architecture/`

它们是 scoped governance / work plans，不因为“存在”就自动成为本 PR 的施工范围。
恢复线程应读取其当前 `tasks.md` 和适用 specs；只有当前用户请求继续/采纳的 scope 才进入施工。

`case-evidence-publication-gate/tasks.md` 的 2026-09-11 update 已接受原 full9.8a 与 shared platform 2.4/4.3/4.4 scope，同时仍明确 actual A0/B1、Product/Formal RC 等为独立未闭合 seam。
不要再从 2026-09-10 handover 的旧措辞推断这些四行仍开放。

## 6. 新线程强制恢复顺序

新的 ChatGPT / Codex / Owner 线程在**第一次写入前**执行以下顺序：

1. 读根 `AGENTS.md`，再读目标目录上的更具体 `AGENTS.md`；文档工作同时读 `docs/AGENTS.md`。
2. 读 `docs/analytix/README.md`、`docs/analytix/handovers/README.md`、本检查点和 `docs/analytix/knowledge-base.md`。
3. 重新获取 GitHub 当前 `main`、目标分支、PR #28、branch HEAD、base SHA 和 merge/draft 状态；不要沿用聊天中的 SHA。
4. 有本地 worktree 时运行 `git status --short --branch`、`git rev-parse HEAD`、`git diff --stat`；没有本地 worktree时，使用 GitHub branch/PR/commit 事实并明确局限。
5. 动态检查 `openspec/changes/`，读取当前请求真正涉及的 `tasks.md`；重新计算未完成分母，不从旧交接继承。
6. 读取当前精确 HEAD 的 CI / CodeQL / review 状态；旧 SHA 的通过只能按相关代码和环境是否未变来限定复用。
7. 若 Notion connector 可用，读取 **Analytix Engineering Workflow & Templates** Skill，并在 **Construction Ledger** 中检索 `Repository = Eysn0130/analytix` 的非 Complete / 非 Superseded 项。Notion 是索引，不覆盖 GitHub。
8. 对比 GitHub、handover、Notion；冲突时保留差异并以 GitHub / accepted spec / fresh evidence 为准，随后修正镜像，不静默覆盖代码。
9. 从“当前用户已授权范围内、依赖已满足、能被验证的下一个 gap”继续；不要因为 handover backlog 存在就自动扩大 scope。
10. 在本线程形成新的 durable checkpoint 时，先更新仓库事实/证据，再同步 Notion；正式报告、表格、PPT 或导出物才进入 Google Drive 交付层。

## 7. 新线程不得做的事

- 不从聊天摘要、Notion、历史 handover 直接宣布当前 PASS；
- 不重做已经存在且仍可信的全部验证，只复跑因代码/环境/覆盖变化而失效的门禁；
- 不为了恢复上下文新建重复分支、重复 PR 或第二套任务账本；
- 不自动把 PR #27 或历史 worktree 的内容混入 PR #28；
- 不使用 reset / force push / broad checkout 覆盖未知用户改动；
- 不为了 CodeQL 通过而扩大 allowlist、关闭规则、降低严重性或跳过测试；
- 不把 Notion 或 Google Drive 当作源码、SHA、CI、release 的权威来源；
- 不把凭据、token、个人/案件原始数据、私有日志复制进 handover、Notion 或公开仓库。

## 8. 每个线程结束前的最小交接合同

只要线程产生了有意义的代码、验收、阻塞或施工顺序变化，结束前至少记录：

```text
Repository:
Branch:
PR:
Base SHA observed:
Start HEAD:
End HEAD / candidate:
User-authorized scope:
Changes actually made:
Checks actually run:
CI / CodeQL / review status:
PASS / FAIL / BLOCKED / UNVERIFIED items:
External/environment blockers:
Exact next dependency-valid action:
Out of scope / explicitly not done:
Notion Construction Ledger item updated: yes/no/pending
Formal delivery exported to Drive: yes/no/not-applicable
```

“Plan”不能代替“Changes actually made”；“命令启动”不能代替最终 exit/result；“正在跑”在换线程时按 RUNNING / RESULT UNKNOWN 处理，恢复后 fresh 查询。

## 9. 知识与交付层

自 2026-09-16 起，项目治理采用：

```text
GitHub repository = 唯一工程事实源
Notion / Analytix Engineering Knowledge Base = 非权威知识、ADR、研究与施工索引
Google Drive / Analytix = 正式报告、Sheets/Benchmarks、Slides、Release/Evidence exports
```

详细冲突、敏感信息和写入规则见 [`../knowledge-base.md`](../knowledge-base.md)。
旧 Obsidian vault 只保留历史/legacy 角色，不再作为 canonical 项目知识层；未经明确新授权不要自动同步或双写。

## 10. 当前下一步

恢复 PR #28 时按以下优先级：

1. 读取当前 branch HEAD 与 PR #28 最新状态；
2. 查询 `analysis_compute` rerun 的最终结果和新 HEAD 的 exact CI；
3. 核对 PR #28 当前 CodeQL / review conversations，关闭仍真实存在的安全/路径边界 gap；
4. 在不依赖锁定 QA 身份的 deterministic 范围内继续完成当前候选验证与必要修复；
5. GUI 连续旅程只在正常凭据/环境 seam 可用且获相应权限时恢复，不绕过；
6. 所有 mandatory gates 闭合后再把 Draft PR 转入 merge-readiness 审核；本文件本身不授权 merge。


## Preserved handover entry at d8640957c — migrated 2026-09-21

Historical only. Original source `docs/analytix/handovers/README.md`, SHA256 `57d367da2f4971651f1627f9ccdd71c674c83700716d2cf566b93379ea4714c2`. Heading labels and product-relative links were relocated; execution results were not revised. Current work resumes through [README.md](README.md).

<a id="snapshot-d864-handover-1"></a>

# Historical d864: Analytix 施工交接索引

Status: Operational index
Applies to: 跨线程暂停、恢复与阶段性交接
Source of truth: 当前代码、当前适用 `openspec/changes/*/tasks.md`、当前 Git/GitHub、当前 PR/CI/review 和确有必要的新验证
Supersedes: 无；本目录不替代 accepted specs、OpenSpec 或代码

本目录只保存“某一时点如何安全接续施工”的操作性快照。交接文档可以说明当时
看到了什么、运行过什么、还缺什么，但不能把历史 PASS 自动继承给新的工作区。
**本页是跨线程恢复的恒定入口**：不要为每个新会话另建第二套总账。

<a id="snapshot-d864-handover-12"></a>

## Historical d864: 当前接续：PR28 A–N 新反例与固定引擎验证 / 2026-09-21 PDT

START `5242782f099f10ddb6af91eb70e8ad5a0944aa1e` clean/ahead57，继续原分支与原writer。
SOURCE `4343ec436ce5df65ba4f413ed49fabab90825a8c` / tree
`2e94577ae8d9bac4adabb53a33f7cf9513193aec`；其后文档DELIVERY由本节提交与最终manifest单列。
[本轮 A–N 实际结果](../qa/pr28-independent-review-execution-2026-09-21.md#current-closure-continuation-an-browser-actions-docx-structure-and-bounded-rust-probes)
与[产品矩阵](../product-completion.md)是恢复入口。保留全部祖先，不从dde重做。

已定向提交Browser显式选区动作及focused-child拒绝、Rust静态阶段诊断/有界对照、
DOCX字段/链接结构保护。实际Electron复现旧parent选区被误取；固定Office复现DATE字段
被setString扁平化，修复后明确unsupported且不导出/提交，保留可取消review。
新源码/测试18文件+314/-28（生产/契约/配置11+82/-23；测试7+232/-5）。
本包19/19；64是审查工具自测，32是逐项覆盖的场景规格，均不是全产品通过数。

Browser59/59、panel4/4、Office42/42+controller41/41以及typecheck/lint/build通过，组间不加总。
Rust原目标1/1和page对照2/2未复现INTERNAL；六次query额度已用，保留初次诊断receipt失败。
固定Office4案例22断言14通过/8失败：无编辑及编辑后重开中文默认字号10.5→11；
这是新的未解决保真问题，不能写成完整DOCX或installed验收通过。
准确源码私有DMG已生成，470395220字节，SHA256
`200a8772dbfe36fc1212684a7b1bb0d183005b0f03a72892d6ba040c18f63338`。
签名、DMG完整性和Office35文件校验通过；包真实分类development_dirty_non_publishable
（当时三份文档dirty），未安装/启动/公证/发布，后续文档commit不把旧artifact改称clean。
Electron Cookie encryption的独立OS存储边界仍未准入，Go task-Keychain绑定不能替代它。

下一最小工作：沿已有显式/省略字号单变量对照研究原codec/worker保真策略；
按现有typed owner继续导入pivot/chart局部编辑；补Skills archive identity/trust/stage/generation
真实权限链；完成图片、Browser及共同主会话的准确候选原生旅程。旧Canvas/图片CAS/
Write清退/主线程补全有效工作不重做。源码缺口、未运行证据和具体拒绝分别记录。

远端只读观察仍dde Draft，main ce96；本地新SOURCE没有对应CI/CodeQL。原create_tree自动
审核拒绝范围未知且未解除，不换push/API/ZIP外发。未merge，真正main未验收。
用户最新目标含条件发布，但现有资源、安装、RC/release gates尚不合格。下一方REVIEW_ONLY，
原writer未release；无Goal、续跑、新任务或并行writer。下文是历史，不驱动重做已完成能力。

<a id="snapshot-d864-handover-46"></a>

## Historical d864: 历史接续：PR28 Browser 与证据复核 / 2026-09-21 PDT

从 clean `40a6a1c9aca56229eb56b43dce381b709a86462b`、tree
`616a867512c4281544b8a6113564b34a56c2a611` 继续原分支、原 writer；保留全部祖先。
[同一 QA 的本轮记录](../qa/pr28-independent-review-execution-2026-09-21.md#historical-continuation-am-review-package-and-browser-implementation)
区分新的16/16+9/9、32工具自测、实际产品测试与固定Electron引擎证据。
Browser 已新增可信 Main guest/document→选区→Core私有currentness→隐私投影→
主会话引用链，不再把整个Browser能力称为零实现。并发捕获与完整scope绑定反例已修复。
WorkspaceStatus #33另有受保护存在性泄漏的真实反例，已沿现有进程隔离owner修复并验证。
源码检查点 `65ab0efca8765385dc09d3522ca567d9bffd0f40`，tree `36370dd74196b8d7e141c5cdcbff45aeb1145eee`。
实际guest产品GUI、child-frame capture/专用快捷动作、导入pivot/chart、固定Office保真、
Skills archive authority/真实安装生命周期、准确候选私有安装仍分别有源码或证据缺口。

QA已更正flow steps/含端点记录计数，恢复旧实际执行回执并保留NOT_RECORDED；
不要重跑未失效995项或重做旧Canvas/R07/图片/数值实现。当前完整A—M仍partial。
远端仍旧dde Draft、main仍ce96，CodeQL/main Rust与Development gate仍失败。
源码外发拒绝未解除；原owner未release，下一方仍REVIEW_ONLY，不通过ZIP/API/push绕过。
SOURCE、最终DELIVERY、tree/dirty与精确增删行以本轮manifest和普通提交记录为准。

<a id="snapshot-d864-handover-65"></a>

## Historical d864: 历史接续：PR28 独立审查落地 / 2026-09-21 PDT

从 clean `fe65622e5d827637f6a7423ef60dc048fee20d3c` 继续原分支，保留148d及全部较晚有效历史。
[本轮执行记录](../qa/pr28-independent-review-execution-2026-09-21.md)与其所在提交是新源码检查点；
新交付manifest另记准确HEAD/tree/dirty及67条当前映射。完成Office/Canvas导航失效修复、
数值与source-read授权/missing-key投影补证，以及最多8区的图片CAS/选择备注/引用/预览旋转。
新增的引用保存竞态、冲突重选CAS与逐区重画回归均已复现修复。

完整A—M仍未完成。Browser可信guest/document→Core引用、导入pivot/chart与固定引擎/安装
实际验收继续区分为源码缺口和证据缺口。不要重做既有单区/Canvas/R07、148d数值修复，
也不要把继承995/1783/prod12写成新图片合同候选全套通过。当前owner保留writer；下一方
REVIEW_ONLY仅收到新增可分享证据，不收到未获准源码。安全拒绝未解除，不以push/ZIP/另一
执行器替代被拒的外发；PR仍Draft、未merge、真正main未验证。Notion仅镜像验证后的摘要。

<a id="snapshot-d864-handover-79"></a>

## Historical d864: 历史接续：PR28 双执行器 / 2026-09-21 PDT

最新源码检查点 `148d188e4f7df71e1cf28bc25bfff988365fa1ea` 保留 `2edb7d463` 及之前历史。
本轮从实际SARIF复现并修复转换前整数范围缺陷；9个Go包995顶层测试通过，生产标签12项通过。
[双执行器执行记录](../qa/pr28-dual-executor-2026-09-21.md)保存准确tree、差异、失败/通过及拒绝范围。
下一方为ChatGPT + GitHub REVIEW_ONLY；原Codex保留source writer，最新源码尚未合法同步，
只交接报告与旧公开SHA的SARIF诊断，不用源码附件绕过外发拒绝。产品全范围和以下剩余事项继续有效。

<a id="snapshot-d864-handover-87"></a>

## Historical d864: 历史接续：PR28 A—M 本地源码推进 / 2026-09-20 PDT

用户采用 `Analytix-PR28-Reaudit-and-Codex-Plan-2026-09-20.zip` 全部 A—M。
从原分支 clean `2ef3f9deb22714b7c05583603bf1e394cce1f06e` 继续，保留全部后续历史。
本次包与下方旧17文件包不同：新包12个普通文件、manifest核验通过；两份原材料已完整读取。
本轮源码检查点为 `5c2f1b089533b70e3a7623396e9690d2d355aab4`；其后文档提交不改变源码。
精确提交、当前矩阵、命令结果和证据上限见
[本轮执行记录](../qa/pr28-reaudit-execution-2026-09-20.md)。恢复时仍先刷新 HEAD/main/PR/CI，
不要退回这里的起点或把历史源码验证当成新候选结果。

本轮已推进 Core 授权 Write 检索、主会话辅助补全及取消、预算整数溢出防护、Skills
发现与消费撤权、真实 pivot package、DOCX oracle、图片区域 Core CAS/引用和有限 PPT
形状 geometry/fill 的 typed/native/整包验证链；没有重做旧 Write 清退、8/4上限、Canvas或R07。
完整 A—M **尚未完成**。原生 DOCX 保真/固定引擎实际三格式旅程、图片多区域与旋转、
浏览器可信 guest/document 引用、导入 pivot/chart 修改、安装版 Skills/Provider/完整 GUI
仍缺实现或实际验收。Rust/DuckDB根因和新 SHA CodeQL 也未关闭。

本机一般开发/测试/GUI授权已具备；源码外发、Chromium管理和SecurityAgent的具体拒绝
仍独立处理。原 `create_tree` 自动审核拒绝缺少解除记录，未换 `git push` 绕过，因此
没有新远端 CI/merge/main验收。用户允许在具体准入与适用gate通过后普通push和正常merge；
旧永久Draft约束不再是当前用户要求。禁止 Goal/自动续跑/新独立任务、强推、main直推、tag/Release。

<a id="snapshot-d864-handover-109"></a>

## Historical d864: 历史接续：完整性复核与缓存资源上限 / 2026-09-20 PDT

本次起点为 clean `33510c813fc3a93bb98c82ad746476bb9399c987`，tree
`486b2e3ca654daa7cfe2c31d0110042cde666ff6`，仍在原施工分支；未回退 b15、重做 Canvas
或重复 R07。ZIP hash 未变，17 个已接收文件逐个与 ZIP 正文比较相同，复用已有安全
接收及完整阅读回执，不重新展开。下方原 16f95 恢复锚点与 32 个继承提交仍保留。

2026-09-21 03:01:52 UTC 重新读取 main/PR28/rules/checks/reviews，main 仍 ce96、
PR head 仍 dde、OPEN/Draft/unmerged，两个 filestore threads 未解决；main 64 条检查
有两项失败、dde 56 条有 CodeQL 一项失败。33510c813 的远端 workflow runs 为 0。
沿原 PR28 是唯一接续路径，不把 merge-test 当 main。

此次补齐 retrieval owner 的 8 个 LRU 索引与 4 个 active build 上限；clear 后未结束
的读操作仍占用构建名额。RED 2 fail/9 pass → 两个最终服务测试文件 28 pass；
新增 canary 验证 retrieval disabled 为零扫描且不进入 runtime intent，enabled 正常读取。
Node 类型检查及三文件 lint 通过；未新跑 native/GUI/安装/Provider。准确最终 SHA/tree
从本节所在提交和 Git 获取，完整 P0—P5 裁定见[产品矩阵](../product-completion.md)。

**完整 A—M 未完成，不能把它描述为只差 push。** 三条清退链已完成源码验证；P1—P3
功能及原生验收、P4 生产关闭/撤权与同主会话补全/Skills 实装验收、P0 安全/Rust 以及
P5 安装/集成仍待完成。旧具体安全拒绝缺少解除记录只阻断依赖操作，不替其他未实施项
背书。下一增量应接现有检索/工作区生命周期并验证主动释放与旧请求失效；原生线路
先取得对应拒绝解除记录再复现，远端源码写入同样不可换接口规避。

<a id="snapshot-d864-handover-133"></a>

### Historical d864: 上轮本机接收与开发路线授权

主路线为本机 Codex Desktop，辅助路线为 ChatGPT + GitHub。用户明确授权在本机
canonical 工作区修改、构建、测试、Electron GUI、合成 Office/PDF/图片、中文 IME、
本地私有安装与相称产品验收；不再要求寻找独立 Mac/VM/云环境。详见
[当前运行手册](../development-runbook.md#development-routes)。下方历史“不使用用户 Mac”
及永久 Draft/禁止正常 merge 的任务限制已被当前请求取代；具体安全拒绝、凭据与
真实数据边界仍独立适用，不能借换执行路线解除。

2026-09-21 02:42 UTC 的只读刷新：PR28 仍 OPEN/Draft/unmerged，远端 head 为
`dde784437dc8563e84066629dd57f4a11fd9acc9`，真正 main 为
`ce96cf12581acfa0e19fae7c6aa9c709371012c8`。main 尚不包含 b15；不得把
`9990dc8885e624d24a08b5e9d64246c948008f3f` merge-test 当 main。dde 的 CodeQL
37 high/两个未解决 review threads 尚未闭合；b15 尚无远端 workflow run。

本机原干净 HEAD `16f95baf484757a2334762ad8c8f2e0dd914b572` 已保存在恢复分支
`codex/reaudit-recovery-20260920-16f95`；在原施工分支从验真 bundle 普通 fast-forward
到 `b15a57f7c3a97c238b9361023753e0aed0ab74c5`，tree
`914fd689f26271fa7377f52dee99f6ae2c9dbcdb`。32 个原提交为继承历史，不是本轮新增；
R07 已在其中，不重复应用，不修改独立 UI Refresh。后续本轮变更与验证见
[产品完成矩阵](../product-completion.md)，准确最终 SHA/tree 读取 Git。

本次安全接收包 17 个普通文本文件，展开 367344 bytes，16 项清单全部匹配；完整读取
01 的 A—M 及 00/02—06。接收 ZIP SHA256 为
`b8f109eee9e7578bec89a4d272232a1d164c46a65e63a01ee7679e857f2392f2`。
保留原包与历史报告，不执行包内脚本，不将历史 PASS 转移到新候选。

<a id="snapshot-d864-handover-160"></a>

## Historical d864: 历史接续：R07 合法后续刷新生命周期 / 2026-09-18 UTC

Status: **R07_FOCUSED_SOURCE_VERIFIED / REMOTE_SYNC_NOT_CLEARED**。本节与本批真实生产
修改、原测试增量一同提交，单父为 `43a3d7d7314773c8f3de6a5d682e88fc60109b32`；准确
新 SHA/tree 以 Git 和交付包 `CANDIDATE.json` 为准。仍沿原分支及 Draft PR28，不重造 Canvas。
已独立核验原完整 161 提交 bundle、6380 源文件字节/Git blob/逻辑模式及外内层清单。

- 原 Workbench 39 项上接入 supplied R07 四项：实跑 40 PASS / 3 FAIL，三项正常索引
  接续失败，真实 A→B→A 取消反例保持 PASS。修复后四项全通过，原 39 项均保留。
- 原 send owner 分离短期回执写权与后续刷新代次；正常 `finally` 仍释放回执令牌，
  不再误杀合法刷新。后续仍核对发送代次、线程、原订阅/abort、busy 和 turn/user 快照；
  只捕获必要原始字段，不永久保留旧 owner 或整个 store。R06 的稳定 ID 兼容未回退。
- 原 navigation 将已有单个 1500ms 索引计时槽归其 action factory，使用有界请求代次；
  新合法读取替代旧槽，过期返回/异常不写回，ready 取消轮询，失效调用不能抢占新任务。
  实测快速断线重连、但新预加载仍未返回时，旧 timer/response 也会复活；两个 RED 已
  在原 runtime-check/offline 路径失效该索引任务后 GREEN，不新增调度系统。
- 最终原 52 文件 818 PASS / 0 FAIL / 0 SKIP；原 Workbench 文件 48 项、navigation
  46 项。另 2 个原缓存/侧栏兼容文件 13 PASS，未计为新增。新增共 20 项：Workbench
  9（supplied 4 + 延迟 I/O 3 + 预热 2），navigation 11。原 798 项全部保留且通过。
  双原 TS 配置、四文件 lint、runtime TS/Electron 源码构建通过。原始 RED/GREEN、
  输入、命令、真实退出码和 fixture 修正保存在交付包；详见[原 QA 页](../qa/canvas-product-loop-2026-09-17.md)。

本批为独立 Linux 非特权源码证据，不是产品完整、原生安装、GUI/IME 或真实 Provider
验收。未变 Go/Rust/锁定输入仅比对，不虚称新跑；不修改 UI Refresh、权限架构或门禁。

本轮 fresh PR28 为 Open/Draft/unmerged，真实远端仍 `dde784437dc8563e84066629dd57f4a11fd9acc9`，
base 为 `ce96cf12581acfa0e19fae7c6aa9c709371012c8`。旧 SHA CodeQL 仍 failure/37 high；
完整 source→sink、可达性和新扫描仍缺。未取得旧出站安全阻断解除证据，因此无源码
出站写入、push/ref/评论更新或新 SHA CI。只读及正式 artifact 下载不构成写入许可。

下一执行者接收新修复全量源码和 bundle，再处理确有许可的同步/精确新 SHA CI及独立
原生资源。不得把 Codex/换接口/转码/拆分/换会话当作解除；不得要求用户运行 Git、
重新导出、传话、提供旧凭据或使用用户 Mac。保持 main/历史/UI 分支不动、PR28 Draft。
唯一远端恢复锚点仍 comment `5709839864`；完整读取并核对正文与 updated_at、保留历史
且获准写入后才更新。本次未读写该评论，不新建第二总账。下方结论均为历史，不覆盖
R07 的修复证据与当前安全边界；无新的确定性本地缺陷时不无限扩大验证或另造 Canvas。

<a id="snapshot-d864-handover-197"></a>

## Historical d864: 历史接续：2026-09-17 回执归属修复 / 2026-09-18 UTC

Status: **RECEIPT_SOURCE_CLOSED / REMOTE_SYNC_NOT_CLEARED**。本节随真实源码、测试一次提交，
单父为 `d4650629f7d030b39430e08e7786e5fa5f938b7d`；准确新 SHA/tree 从当前 Git 和交付包
`CANDIDATE.json` 读取，不另造自引用或纯状态提交。仍为原分支 / 原 Draft PR28。
完整旧 bundle、160 提交、6380 跟踪文件和 42 项候选字节/模式均重新核验；不重建 Canvas。

- 已将原报告三项真实 `selectThread('b')` → `selectThread('a')` 回归并入原 Workbench
  测试，实际 23 PASS / 3 FAIL → 同 26 项全部 PASS。修复原发送 owner 的临时操作令牌、
  订阅身份与运行轮次写权，不以线程 ID 相同授予旧成功/失败回执覆盖新快照的权限。
- 原提交已确认仍返回成功、只清原快照；旧失败不得回滚新状态。合法 SSE 在 HTTP 前
  规范化临时 ID（含 HTTP 不带 user ID）、完成、plan、Office、草稿晚输入均保持。
  stream 迁移、watchdog、completion poll 及 refresh 后续 I/O 同样受归属约束。
- 扩展测试又实际复现旧回执触发的列表 refresh 在 A→B→A 后写回；修复既有导航 owner
  的异步边界和它自己的 loading 标记，不建立第二发送状态机或长期映射。
- 当前 52 实际测试文件 / 798 PASS / 0 FAIL / 0 SKIP；其中原 Workbench 23 项保留、
  该文件共 39 项。本轮新增 26 项（3 个原诊断 + 23 个配对兼容/副作用检查），不把
  额外纳入的 40 个既有测试或旧 732 PASS 计为新测试。双 TS 配置、定向 lint、runtime
  与 Electron main/preload/renderer 源码构建通过。确切命令、输入及失败分类见
  [原 QA 页新增记录](../qa/canvas-product-loop-2026-09-17.md) 与交付包。

**安全边界优先于下方历史同步路线**：本轮未取得原 OpenAI `create_tree` 安全阻断已
解除的证据。没有普通 push、Git Data/contents 替代代码写入或新 SHA CI；不拆分、
改编码、换接口/会话规避检查。原远端仍 `dde784437dc8563e84066629dd57f4a11fd9acc9`，
base `ce96cf12581acfa0e19fae7c6aa9c709371012c8`。受支持的只读/依赖下载不是同步许可。
恢复必须先证明合法获准通路，保留准确出站检查与原 pre-push；否则保存新候选。

当前确认的 R06 及相邻回归已在独立 Linux 源码范围闭合；交付新 bundle 给 Codex 的
是合法同步/新 SHA CI、安全完整证据与独立原生资源边界，不是重新开发 Canvas。
旧 CodeQL 37 high 未消除，Go/Rust 本轮未改未重跑；旧 Go 结果仅历史证据。
不使用用户 Mac/旧凭据/真实案件；GUI/IME/共同 Office—Canvas—图片旅程、真实 Provider/
media、RC、签名公证仍未验收。Codex 也不得绕过相同安全检查或默认拥有原生资源。

唯一远端锚点仍是 comment `5709839864`；更新必须完整读取并保留原正文。不能在只取得
截断正文时覆盖历史，也不能新建第二总账。下方内容为原时点历史，不覆盖本节边界。

<a id="snapshot-d864-handover-233"></a>

## Historical d864: 历史接续：2026-09-17 实际发送链与案件授权组合

Status: **VERIFIED_SOURCE_BATCH / READ_FRESH_REMOTE_STATE**。本节与真实源码和测试一起提交，
不另造状态提交。继续原分支 `codex/workbench-product-delivery-20260914` / Draft PR28。
来源候选 `de709d9c322948e786f87b84fffede3d9f988296` 已实际收到，37 文件、完整父/tree/
blob/模式核验通过。旧未提交候选仍未收到，但用户已取消把它或旧停写回执作为本轮前提。
后来真实到达的改动仍须保护；不修改独立 UI Refresh，不 merge/rebase/force/main/release。

- 实际 Workbench/plan/send store/引用与草稿 owner 的 17 项初始回归发现 6 项失败，
  原 owner 修复后同 17 项全部通过；扩展为 23 项真实消费链测试。覆盖最后异步 I/O
  后 Core 复核、拒绝保留、busy 不排队临时授权、迟到回执/错误线程隔离、重复准备、
  只清理已提交快照及 Office 原路径。没有复制测试专用 handler 或新建 Canvas。
- 真实签名案件 Registry、Host risk-policy CAS、case binding、epoch、原 Host/projector/
  Registry/filestore 验证普通案件工作区文件的合法修改与失效拒绝；4 顶层、7 子场景。
  该组合明确不授予 DSV2 案件事实发布，不冒充 independent-witness 或完整案件 AI 验收。
- 本轮完整 UI 回归 43 实际文件 / 732 PASS；双配置类型检查、当前 lint、runtime TS 和
  Electron 源码构建通过。Go 当前 normal/prod/race 精确结果及命令见
  [同批 QA 记录](../qa/canvas-product-loop-2026-09-17.md)。新旧、重复、嵌套计数分别保留。
- 实际 Chromium 首次访问合成 loopback 验收页被 `ERR_BLOCKED_BY_ADMINISTRATOR` 拒绝。
  未绕过策略，没有浏览器 UI PASS；源码/组件证据不替代原生安装、GUI、IME、Provider。

**恢复顺序**：读取当前 Git/GitHub 原 ref/PR 和 comment `5709839864`，以及交付包实际
commit/tree/manifest，确认真实写入结果。终端 DNS 失败不等于连接器只读；完整出站检查后
【历史同步方式说明，非当前准入】本句记录当时的技术路线，不解除其后具体create_tree拒绝。
受该拒绝影响的源码不得换普通push、GitData/contents API或ZIP外发；当前先依实际拒绝范围及正规审批处置。
具体准入成立后，由原owner按当前协议普通push原分支，记录准确HEAD/tree并核验新的CI。
若 API 生成等价 tree 的新 commit，准确记录新 SHA，不能声称原 de709 字节已原样推送。
没有 ref 前移就没有 push；在新 SHA 收口 CI，不借用 dde 的 51-job 历史成功。
只有实际动作证明必要通路/合规资源不可用，且不依赖该缺口的工作完成后，才把保存的
最终实际候选及准确剩余项交 Codex；下方旧“下一步 Codex”不覆盖这项接续要求。

37-high 完整 traces/复扫、历史 DuckDB inline 根因、原生共同对象旅程、授权真实模型/
媒体、安装、A0/B1/ProductRC/FormalRC、签名公证仍逐项核验。当前 PR comment 与外部
执行胶囊记录实际远端 SHA/CI/写入边界；本索引不把工具发现、setup 或旧 PASS 变成验收。

<a id="snapshot-d864-handover-268"></a>

## Historical d864: 历史本地候选：2026-09-17 Canvas 引用、持久审阅与消费者闭环

Status: **LOCAL_SOURCE_CANDIDATE / NOT_PUSHED**。源代码、测试和本节在同一候选批次；
准确候选 SHA/tree 在随本轮交付的机器清单和当前 Git 中读取，不为自引用另造提交。
原分支 `codex/workbench-product-delivery-20260914`、PR28 Draft 保持不变；
最后远端核对仍为 `dde784437dc8563e84066629dd57f4a11fd9acc9`。

用户已经明确允许：不再以取得原未提交 Canvas 候选作为本轮独立源码施工前提，
从实时核实的已提交基线补齐功能，交付可回放源码候选，再由 Codex 审阅整合。
本轮没有取得、删除或冒充恢复那批旧候选，也没有声明控制原环境或原 writer。
这项授权不表示远端已经推送，或允许覆盖后来到达的代码与独立 UI Refresh。

- 从准确 `dde` 基线复用原 Go Core/Host/Registry/projector/filestore/Composer，
  完成 Canvas 选择引用、短期模型安全 scope、有限提案、真实差异、明确接受、CAS、
  原操作只读查询、持久审阅重开、撤销/恢复与有界两对象预览消费者；PNG 保持本地
  有限裁剪/旋转/标记，不冒充真实媒体生成。
- 已验证的源代码闭环、协议/隐私/存储边界、命令、失败及剩余门禁，见
  [本批 QA 记录](../qa/canvas-product-loop-2026-09-17.md)。
- 独立 Linux 非特权执行：受影响8包 normal 979顶层PASS、prod 992顶层PASS，
  各944嵌套子测试PASS、4既有SKIP、0FAIL；race 41顶层/91子测试PASS、0SKIP；
  前端/Main/相邻Office/契约/Composer 437PASS、30实际文件、0SKIP。
  不叠加不同模式、重复运行或顶层与子测试。不将其记为远端CI、GUI或安装验收。
- 双配置typecheck、16文件ESLint零警告、runtime TS与Electron源代码构建、Linux
  Go production runtime编译通过。未执行npm lifecycle/native安装或放宽完整构建门禁。
- 对相同旧基线的权限失败已经验证为测试环境umask0022使私有根不满足0700；
  改执行环境umask0077后通过，原断言未动。并发长上下文诊断超时保留；顺序完整
  server normal/prod均通过。细节和既有4跳过见QA记录。

**下一有效动作**：Codex先校验源码包manifest/patch/bundle与此基线，读取实时GitHub
和实际工作树，按真实diff保全所有不同来源的修改；仅整合本批，定向验证后普通非强制
push原分支，保持原PR28 Draft。若远端或工作树已变化，先审阅具体重叠，不整包覆盖，
不自动merge/rebase/cherry-pick另一任务，不复用旧测试替代变化部分的新证据。
本地源码候选并不依赖再次取回旧未提交候选；后来找到它时按真实代码去重和审阅。

原远端恢复锚点仍为comment `5709839864`；此次未更改评论或远端仓库。
新SHA的必需CI、安全告警37high的完整路径与复扫、DuckDB历史间歇性问题、独立原生
GUI/IME/安装、Provider/媒体、A0/B1/ProductRC/FormalRC等均未由本轮宣告闭合。
未访问用户Mac、Keychain、旧profile、凭据或真实案件；Notion/Drive没有替代仓库。

<a id="snapshot-d864-handover-307"></a>

## Historical d864: 历史检查点：2026-09-17 独立构建环境与回执授权边界

仍使用 [PR28 comment 5709839864](https://github.com/Eysn0130/analytix/pull/28#issuecomment-5709839864)
作为唯一活动恢复锚点。恢复时 fresh 查询原分支、PR、该评论和准确候选的 checks/jobs；
本节不将历史 PASS 自动转记给新 HEAD，不授权合并或发布。

本轮实际起点为 `d57b5282fccad025b5f4812a17056527980662c9`。
已推送配置提交 `0bc900e98d9f7152c902481776eaac49777273fb`，真实父提交为 d57b。
本节与回执修复和测试原子提交；包含本节的实际代码 SHA 从 GitHub 读取，不为自引用再提交。

<a id="snapshot-d864-handover-317"></a>

### Historical d864: 已落实的构建依赖

- 新增 `.github/workflows/pr28-independent-environment.yml`，配置 1 文件 +141/-0。
  使用现有标准 Ubuntu runner、固定 action SHA、只读权限、无持久 checkout 凭据；
  限原分支、20 分钟、1 天产物保留，只导出准确公开 HEAD 的祖先对象及锁定工具输入。
  原 Development CI、CodeQL、路径过滤、依赖锁和门禁不变，无新付费设施或 Provider 调用。
- `35209039144` attempt1 在 0bc 上 SUCCESS。源代码 artifact `10490972138`，
  Go `10491192006`，Node `10491477101`，Rust `10490689192`；摘要及恢复步骤在原评论。
  全部外层/内部 SHA256、ZIP CRC 和依赖输入摘要校验通过。产物过期时仅按实际需要
  重运行既有取源作业；不得以重试替代测试根因修复，也不得回退当前分支。
- 独立 Linux 的完整 tracked checkout 已建立：0bc tree
  `0dfcaa5bae4caea214d1804c57cc5d8e8dd020bf`，6,363 个 blob/文件模式、157 个真实历史
  提交、唯一公开根和 `git fsck --full --strict` 均通过。不是四文件索引或重造父历史。
  Go1.26.4、Node22.22.1/npm10.9.4、Rust1.94.1/cargo1.94.1 实际运行且输入与锁一致。
  npm lifecycle 脚本未在取源中执行；完整源码不等于原生安装、合法资源和 GUI 已验收。
- 配置静态/四段 shell 语法检查、7 项合成 Git 导出正常/拒绝测试通过；这些是环境验证，
  不是产品功能或模型测试。终端 DNS 仍不可用，连接器读写可用，两者不能混为权限问题。

<a id="snapshot-d864-handover-335"></a>

### Historical d864: 本批真实产品修复与验证

- `canvasediting.Service.Apply` 的 pending/applied 重复点击分支，在原 operation 的
  Status 返回后、暴露回执或改变 proposal 状态之前，再校验 principal、scope 和取消状态。
  撤权/取消只返回原 operation ID、UNKNOWN 和 ErrUnavailable，不返回磁盘 revision 或
  committed 状态，不再次写盘；正常重新授权仍能查询该原操作。保留已有 current.id、CAS、
  文件检查与唯一 Core，不改变跨层 API，不声称关闭 CodeQL 告警。
- 生产 `packages/runtime-go/internal/app/canvasediting/service.go` +6/-0；新增测试
  `packages/runtime-go/internal/adapters/outbound/filestore/canvas_receipt_test.go` +137/-0。
  一个顶层测试含 8 个子场景：applied/pending × 合法/撤 scope/撤 identity/取消。
  使用真实 object service、filestore、文件 CAS 与 Registry；包装器仅模拟丢失回执和
  Status 返回后的授权变化，授权 projector/identity 为可控测试边界，不冒充完整隐私链。
- 原实现 RED：8 子场景 2 PASS / 6 FAIL，exit1；修复 GREEN：8 PASS，exit0。
  非特权 uid1000、独立 HOME/TMPDIR、Go1.26.4、GOPROXY=off 下，完整受影响两包
  normal/prod 各 268 顶层 PASS、4 既有 SKIP、0 FAIL，exit0。两种模式不重复计数。
  `go test -race -count=1 -p 1 -parallel 2 -json ./internal/app/canvasediting
  ./internal/adapters/outbound/filestore -run 'TestCanvas'`：9 顶层 PASS、35 子场景 PASS、
  0 SKIP，exit0；子场景内含于顶层，不相加。均在 `packages/runtime-go` 执行。
  完整包命令去掉 race/run 过滤，production 模式增加 `-tags analytix_prod`。
- 初次 root 包测试 267 PASS / 4 SKIP / 1 FAIL：root 可读 chmod000，导致
  `TestCaseBindingObserverClassifiesUnreadableFile` 失败。原失败保留，改用非特权测试身份，
  未改用例或断言。4 既有跳过为 BundledCodecPackageContract、FilesystemAliasesShareReceiptBinding、
  OfficePackageSavedSyntheticFixtures、CreatePlanAutoSelectionDoesNotOverwriteFilesystemAliases；
  当前 Canvas/capture/receipt 测试无跳过。不得据此声称零跳过或完整安装 Office 验收。

<a id="snapshot-d864-handover-360"></a>

### Historical d864: CI、安全和 DuckDB 的真实边界

- d57b 的 Development `35203579095` attempt1 已完整分页：51/51 作业 SUCCESS，
  包括 gate `105160409258`；0bc push 未取消这些已结束作业。两个 Go 日志
  `105144536417` / `105144536403` 确认实际 checkout
  `26a2e0ee820bbd743a57779f8aa93b523040f826`，并确实选择 Canvas/filestore 测试。
  303/297 是 package 分区数量，不是用例数。此证据复用，不重复全量执行旧测试。
- d57b CodeQL check `105144046012` 为 FAILURE、37 high/37 annotations；旧“缺少三个
  配置/NEUTRAL”只保留为历史中间观察。完整 source-to-sink 尚未取得，未关闭或 dismiss。
  0bc Development `35209046530`、dynamic CodeQL `35209040822` 及本批新候选必须 fresh
  核验；环境取源 SUCCESS 和旧 d57b 开发测试通过不能替代它们或安全准入。
- 在准确锁定 Rust/DuckDB 依赖、未改 Rust 源码的 0bc 独立环境，原失败定向复现：
  `cargo test --offline --locked --manifest-path tools/analysis_compute/Cargo.toml
  --test stats_query_cli output_file_cases::query_stats_rows_cli_writes_result_payload_to_output_json
  -- --exact --nocapture`，exit101，同样 INTERNAL index0/vector-size0，1 FAIL。
  堆栈明确在 `tests/stats_query_cli/output_file_cases.rs:16` 的第一次 inline 调用；第二次
  output-json 调用未执行。路径为 `query_rows.rs:124` → `session.rs:319` 的范围非空
  COUNT 检查 → `duckdb_utils.rs:174`。这不是输出文件写盘失败，也未完成最小复现或根因修复。
  不加 SQL 猜修、重试、串行、假计数、skip 或弱化 coverage 条件。原失败日志仍保留。

<a id="snapshot-d864-handover-380"></a>

### Historical d864: 下一依赖有效动作

继续原 CanvasHost/Adapter、Core scope/projector/capture、native selection 工具和原 Composer
引用机制。当前 Canvas 仍缺 model-selection-read/propose/capture；现有原生工具仅由 Office
实现对应分派，schema 的 parts/workbook 不等于 Canvas ops。预览切换/隐藏会关闭 Core
会话，未接受 proposal 的内存生命周期需要与引用、折叠、第三对象回收一起闭合。
不得只加引用按钮或拼原始 Scene 到模型；引用不自动发送，草稿/附件保全，未知结果查原操作。
先补有界选择、真实投影和跨线程/epoch/撤权正反例，再原子贯通接受/CAS/重开/撤销的消费者。

DuckDB 下一步针对 inline 的实际 SQL/计划做最小复现；CodeQL 需完整路径和流证据后按共同
根因修复并复扫。完整 Canvas 主会话链、Office/图片共同旅程、独立 macOS ARM64 安装、
原生 GUI/中文 IME、合法资源、当前受保护 Provider 授权/费用和 Product/Formal RC 均仍未验收。
不依赖用户旧 Mac，不访问旧 Keychain/profile/案件/凭据；Notion 非权威镜像不阻塞 GitHub。

<a id="snapshot-d864-handover-394"></a>

## Historical d864: 历史检查点：2026-09-17 Canvas 受控写入修复

当前活动恢复入口为 [PR28 comment 5709839864](https://github.com/Eysn0130/analytix/pull/28#issuecomment-5709839864)。
先 fresh 读取该评论、PR refs/checks，再读下方历史快照；旧评论和旧 PASS 不覆盖它。
分支仍是 `codex/workbench-product-delivery-20260914`，PR28 Open / Draft，未合并。

本轮代码基线 `57d1add9960f2f72ab926dd34695ac2f5b778d8a`；已非强制推送
`0f4c7af4c4e0d0f2b4057069ff81d6bf9f7fd520`。本页为后续文档提交，包含本页的
提交与最新 HEAD 从 GitHub 获取，不把代码候选检查自动转记到新 HEAD。

- 修复 `canvasediting.Service` 的 Apply / RecoverOperation：managed capture 的
  session-ID 参数使用 Core 的 `current.id`，不再误传绝对 workspace 路径。
  原路径、重新授权、CAS、回执与释放检查不变；不是 CodeQL 告警关闭证明。
- 三个代码文件 +187/-17：生产 1 文件 +2/-2；测试 2 文件 +185/-15。
  真实 Registry 的 18 个成对场景覆盖合法 apply/undo/resume、跨线程、旧修订、
  撤权、硬链接和持久化失败。既有 Canvas/PNG 文件 CAS/重启/撤销测试改用真实
  Registry，去掉忽略 session 参数的宽松替身；未新增依赖或放宽门禁。
- 本地 Go1.23.2、独立 Linux、23 包标准库源码闭包，不是完整当前 checkout。
  新场景对原实现 12 PASS / 6 FAIL，修复后 18 PASS；相关 5 包共 23 个顶层测试
  PASS、0 SKIP，race 同样通过，重复运行不重复计数。持久化 spy 不冒充真实写盘。
  module-mode 调用因仓库要求 Go >=1.25 而 BLOCKED；完整 filestore 与当前 CI
  的真实结果仍须查询。历史快照 6,341 blob 校验不代表完整新 HEAD 可构建。
- 基线 `35197821626` attempt1 的 51 作业已完整分页：46 SUCCESS、2 FAILURE、
  3 CANCELLED。实际 checkout `e29cef4f857dd334fc446ff016343d0fc293fe60`。
  data_engine `105125131513` SUCCESS；analysis_compute `105125131323` FAILURE：
  `output_file_cases::query_stats_rows_cli_writes_result_payload_to_output_json`
  遇 DuckDB index0/size0 INTERNAL 错误。没有最小复现，不猜测 SQL 修法；原失败保留。
- 代码候选 Development `35202453830`、CodeQL check `105140340461` 必须 fresh
  核验；后者初次返回缺少三个配置的中间状态，不是通过。此前 37 high 未关闭。
  当前连接器拒绝完整 code-scanning alerts 端点，不能据此认定无告警。
- 下一步先核对准确 HEAD 的 Go/filestore 和完整 CI，再复用现有 CanvasHost、Core
  objectediting scope/projector 与 native selection 工具补齐安全引用到同一主会话。
  Selector 必须重新捕获，引用不自动发送，未接受不写盘；不另建权限/提交体系。
  同项目重新授权正例、旧线程句柄拒绝、草稿保全、审阅/CAS/恢复都仍在验收范围内。
- 完整 Canvas 对话链、Office/图片共同旅程、原生 GUI/中文 IME、独立 macOS ARM64
  安装、真实 Provider/媒体与 Product/Formal RC 仍未验收。终端 DNS、工具链和独立
  原生设施/授权缺口分别记录；未访问用户 Mac、Keychain、真实 profile 或案件。
  Notion mirror: PENDING；GitHub 可独立恢复。无 main/force/merge/tag/release 或新付费设施。

<a id="snapshot-d864-handover-433"></a>

## Historical d864: 最新保留快照

- [`2026-09-17-round2-integration.md`](2026-09-17-round2-integration.md)：
  Round2 代码已按两批集成至 `d7f542ab…`，保留 Round3 隐私修复；含本轮计数、
  独立复验、CI/CodeQL 证据边界及当时恢复锚点；按需读取，不覆盖上方新检查点。

- [`2026-09-17-round3-privacy-projection.md`](2026-09-17-round3-privacy-projection.md)：
  新增 Go 隐私投影修复、精确金额与受信任协议边界回归、受限环境验证及下一步。
  当时 Round2 未集成的状态已由上方集成检查点更新；其余证据保持原有范围。

- [`2026-09-17-knowledge-acceptance.md`](2026-09-17-knowledge-acceptance.md)：
  三层知识治理验收、Round2 本地候选原件归档、SHA/交付清单与线程接续补充。
  与下面的 PR #28 产品检查点一起读取；归档完成不表示补丁已集成或产品通过。

- [`2026-09-16-pr28-continuation.md`](2026-09-16-pr28-continuation.md)：
  当前 Draft PR #28、`codex/workbench-product-delivery-20260914`、精确候选 CI、
  CodeQL/review、GUI 外部阻塞、Notion/Drive 知识治理和新线程恢复协议。恢复当前
  workbench / native annotation 施工优先读它；其中 SHA 与 CI 仍须 fresh 核对。
- [`2026-09-10-owner-replacement.md`](2026-09-10-owner-replacement.md)：
  新 Owner 替换、R131/B1 同源链聚焦接受、无自动化 Slice 路线与当时 A0/B1
  缺口。自 2026-09-16 起它不再是最新施工入口；仍保留当时证据边界和历史恢复背景。
- [`2026-08-05-damaged-cache-retirement.md`](2026-08-05-damaged-cache-retirement.md)：
  当时 clean canonical 基线、损坏缓存退役、v3 保全与容量状态。仅保留该日期的
  缓存和恢复历史，不是当前 HEAD、CI、writer 或施工顺序的依据。
- [`../document-consolidation-register.md`](../document-consolidation-register.md)：
  项目文档分类、碎片归并和历史文档治理登记。

<a id="snapshot-d864-handover-460"></a>

## Historical d864: Historical

- [`2026-08-04-controlled-thread-checkpoint.md`](2026-08-04-controlled-thread-checkpoint.md)：
  当时的 canonical 收敛、缓存容量与 Milestone A/B gap；其中旧镜像保留和容量
  blocker 状态已由 2026-08-05 checkpoint 取代。
- [`2026-07-25-thread-close-route-audit.md`](2026-07-25-thread-close-route-audit.md)：
  当时的产品路线复核、Codex/Claude Code/OpenCode 对照、Obsidian 审核和
  上游 currentness；不替代当前知识治理或 2026-09-16 checkpoint。
- [`2026-07-25-first-stage-pause.md`](2026-07-25-first-stage-pause.md)：
  第一阶段施工暂停时的工作区、Goal、P0-P4、资金插件、上游吸收、验证和风险
  快照；仅作 Historical 背景。

<a id="snapshot-d864-handover-472"></a>

## Historical d864: 新线程恢复顺序

1. 先读取当前目标 PR 元数据，并在其 fresh head 分支读取根 `AGENTS.md`；
   文档工作再读 `docs/AGENTS.md`。读取 `docs/analytix/README.md`、本页、
   当前产品检查点及其治理/归档补充和
   [`../knowledge-base.md`](../knowledge-base.md)。
2. fresh 获取当前 `main`、目标分支、PR、Base SHA、HEAD、Draft/merge 状态和
   current checks；有本地 worktree 时再执行 `git status --short --branch`、
   `git rev-parse HEAD` 和 `git diff --stat`。
3. 比对交接记录与当前 Git/GitHub；任何差异都按新的事实处理，不覆盖用户修改，
   不把旧候选 SHA 的 PASS 自动继承给新 HEAD。
4. 动态查看 `openspec/changes/`，读取当前用户授权范围涉及的实际 `tasks.md`，
   重新计算任务分母和完成数。active change 存在本身不扩大当前施工范围。
5. 检查当前精确 HEAD 的 CI、CodeQL/review 和必要的外部环境 seam。对“上次命令
   仍在运行”“结果丢失”“未复跑”“外部环境缺失”分别标记，不改写成 PASS。
6. 如果 Notion connector 可用，读取 **Analytix Engineering Workflow & Templates**
   Skill，并检索 **Construction Ledger** 中 `Repository = Eysn0130/analytix` 的
   非 `Complete` / 非 `Superseded` 项。Notion 是索引；与 GitHub 冲突时先保留
   差异，再按仓库、accepted target 和 fresh evidence 修正镜像。
7. 只为当前批准的施工切片建立验证计划；从依赖已满足、可验证的下一个 gap 继续。
   交接中的 backlog、旧 PR 或历史 OpenSpec 行不自动进入范围。
8. 一个线程产生 durable 改动、验证结论、阻塞或 next-action 变化时，先更新仓库
   事实/证据，再同步 Notion。Google Drive 仅用于正式报告、表格、PPT、release /
   evidence export，不承担当前施工状态。

<a id="snapshot-d864-handover-497"></a>

## Historical d864: 交接最小合同

每个会话只有在状态发生有意义变化时才需要新 checkpoint；不要为无变化的读取建立
大量日志。新 checkpoint 至少能回答：

| 字段 | 必须回答的问题 |
| --- | --- |
| Repository / Branch / PR | 正在改哪里，是否还是同一条施工线？ |
| Base SHA / Start HEAD / End HEAD | 本轮从哪里开始、实际交付到哪里？ |
| Authorized scope | 用户本轮实际允许做什么？ |
| Changes actually made | 真正修改了什么，不是计划做什么？ |
| Checks actually completed | 哪些命令/CI 最终有结果？ |
| CI / CodeQL / review | 当前候选还有哪些外部门禁？ |
| PASS / FAIL / BLOCKED / UNVERIFIED | 每个关键 seam 的真实状态是什么？ |
| Next dependency-valid action | 新线程第一项应该做什么？ |
| Out of scope | 哪些明确没有做，避免新线程误补？ |
| Notion mirror | Construction Ledger 是否已同步或明确 pending？ |

<a id="snapshot-d864-handover-515"></a>

## Historical d864: 交接状态词

| 状态 | 含义 |
| --- | --- |
| `PASS` | 在记录的工作区、SHA 和环境中实际执行并通过。 |
| `FAIL` | 实际执行且失败，结果仍有效。 |
| `ABORTED` | 人工终止或因范围冲突终止，不是产品失败证明。 |
| `RUNNING` | 检查仍在执行；不得推断最终结果。 |
| `RESULT LOST` | 命令曾运行，但无法取得最终退出状态或完整结果。 |
| `UNVERIFIED` | 没有满足真实环境、凭据、主机、数据或重新执行条件。 |
| `NOT IMPLEMENTED` | 代码路径或生产组合尚未存在。 |
| `BLOCKED` | 明确前置条件未满足，无法进行该项验证。 |

不得用文件存在、fixture 通过、进程启动、历史报告标题含 `final`、旧 SHA 的绿色
检查或 active OpenSpec artifact 已生成来替代产品级验收。

<a id="snapshot-d864-handover-531"></a>

## Historical d864: 单一事实链

跨线程连续性采用以下单向关系，不建立自动双向同步：

```text
GitHub current code / specs / PR / CI / evidence
                    ↓
        handovers（恢复索引与时点快照）
                    ↓
Notion Construction Ledger / ADR / Research（非权威结构化镜像）
                    ↓
Google Drive formal deliverables（需要时才输出）
```

聊天记录可以帮助解释。GitHub可独立恢复的范围仅包括已合法同步的代码、文档和证据。
未同步的本机65ab/5242及其有效后继，需要原owner的准确本机checkpoint与合法可用证据；
远端仍是dde时，不得据旧GitHub重建或覆盖较新工作。Notion仅为镜像，不能补足未提供的源码。


## Preserved product entry at d8640957c — migrated 2026-09-21

Historical only. Original source `docs/analytix/product-completion.md`, SHA256 `c2d7429c6fc7a389587535ef0a606e322bed94704ac7461b4339144491aa881e`. Heading labels and product-relative links were relocated; execution results were not revised. Current work resumes through [README.md](README.md).

<a id="snapshot-d864-product-1"></a>

# Historical d864: Analytix product completion

Status: Operational. This is the current capability and evidence matrix for the
user-authorized product delivery, refreshed on 2026-09-21 PDT. It does not declare
product acceptance, change licenses, or authorize public release. The accepted
outcome includes a lawful installed macOS ARM64 application, not only source.
Historical QA remains valid only for its recorded candidate and environment.

One main conversation must support generation → native preview → selection and
explicit quick actions → annotation → proposal → review → controlled application
→ reliable reopen/recovery. Passive selection and quoting never send a task;
explicit task actions preserve the existing composer draft and attachments.
The Go runtime remains the sole production agent and authority. Plugins and
format adapters cannot replace permission, privacy, Provider or persistence policy.

<a id="snapshot-d864-product-16"></a>

## Historical d864: Current capability and evidence matrix

The A–M implementation batch starts at `2ef3f9deb22714b7c05583603bf1e394cce1f06e`
on the original PR28 branch. Its exact source checkpoint, P0–P5 matrix and fresh
verification are recorded in the [2026-09-20 A–M execution receipt](../qa/pr28-reaudit-execution-2026-09-20.md).
The [handover index](README.md) remains the resumption entry.
The [2026-09-21 dual-executor receipt](../qa/pr28-dual-executor-2026-09-21.md) records
the newer source checkpoint `148d188e4`, integer-boundary fixes, actual SARIF mapping,
and REVIEW_ONLY handoff. Native/product gaps below remain open; no new remote CI or merge is claimed.

The [independent-review continuation receipt](../qa/pr28-independent-review-execution-2026-09-21.md)
records the newer current-source evidence: Office/Canvas document-lease repairs,
retained 148d numeric fixes with focused coverage, missing-key projection and
read-before-authority tests, and image collections with stable region identities,
whole-collection CAS and display rotation/zoom. Images remain discussion-only;
non-normal EXIF/animated PNG and unauthorized pixel editing remain fail closed.

Core-authorized retrieval, primary-thread auxiliary completion, exact subagent
budgets, skill discovery/revocation, real pivot generation, the synthetic DOCX
oracle and bounded PPT geometry/fill remain integrated. Native image acceptance,
imported pivot/chart editing and the recorded fixed engine/installed journeys remain open.
The continuation from `40a6a1c9a` integrated Main-owned Browser selection capture,
Core currentness/privacy projection and opaque same-thread references at `65ab0efca`;
it also repaired the reproduced WorkspaceStatus protected-metadata leak.
The latest A–N continuation starts at `5242782f0` and delivers SOURCE
`4343ec436ce5df65ba4f413ed49fabab90825a8c`, tree
`2e94577ae8d9bac4adabb53a33f7cf9513193aec`. Browser now has an explicit Explain
selection action through the existing primary-thread submit owner. Focused child
selection is explicitly unsupported; real Electron demonstrated that the old capture
could return stale parent text. Passive quote remains non-sending, and held-submit
revocation preserves drafts/attachments. Actual guest/Core/installed journey remains
unaccepted. Child-frame capture itself is not implemented.

Fixed Office execution reproduced DATE-field flattening in the old text-range owner.
The current bounded captured-range guard rejects fields, links and unknown structure
before mutation, retaining a cancellable review without export/commit. Plain and
cross-run text remain admitted. Focused worker/controller checks pass. However, the
actual pinned-engine matrix has14 passing/8 failing assertions: no-op and edited
export/reopen change default Asian font height10.5→11. Structure checks preserve table
and hyperlink relationships; complete typography/visual/native fidelity is not accepted.
A subsequent explicit11-point run-size control remains stable, while the unchanged
omitted-size fixture reproduces the drift; original/no-op renders were visually
inspected. This narrows the issue without making the failing matrix pass.
This new observation supersedes the earlier setString-risk-only hypothesis for fields;
the font-default drift has no proven setString root cause or implemented repair yet.

Rust's unchanged original CLI target passes once, and two fresh-session page controls
pass after diagnostic receipt registration was corrected. Six target/query executions
were used, with no INTERNAL reproduced; this does not close the old main failure.
Current-candidate CodeQL/remote CI have not been run. Imported pivot/chart editing
remains a source gap. Desktop Skills archive installation still fails closed until
real archive trust/identity and Go materialization authority are integrated; filesystem
materialization tests do not prove that absent install/uninstall route.
The accurate-source private DMG now builds successfully after preserving and
repairing one corrupt Go module-cache ZIP through normal checksum-verified download.
DMG SHA256200a8772dbfe36fc1212684a7b1bb0d183005b0f03a72892d6ba040c18f63338,
470395220 bytes; signature, DMG integrity and Office35-file content checks pass.
Its immutable classification remains development_dirty_non_publishable (three
uncommitted documentation files at build); it is not installed, notarized or released.
Electron Cookie encryption initializes OS key storage before Core: the current
non-login Go task-Keychain binding does not supply an admitted Chromium storage
boundary. That source/admission gap remains separate from the private artifact build.

See the current A–N section of the same QA for all32 scenario dispositions,67 finding
mappings, exact receipts, failures and remaining P0–P5 scope. Reviewer64 self-tests
are not product tests;32 specifications are not32 passes. Earlier matrices are history.

At the latest read, main remains `ce96cf12581acfa0e19fae7c6aa9c709371012c8`;
PR28 remains OPEN/Draft at `dde784437dc8563e84066629dd57f4a11fd9acc9`.
Specific outbound and native safety refusals remain separate from the already
granted local Mac authorization. **The complete A–N continuation and original A–M/P0–P5 remain partial;
main merge, main acceptance, full product acceptance and Formal RC are not achieved.**
At the owner-recorded checkpoint, public release had not been authorized or accepted.
The latest user request now includes final product publication as a conditional delivery goal.
It does not clear the specific source-sync refusal or qualify existing non-publishable resources;
publication requires current artifact admission, licenses/notices, exact-candidate acceptance and release-gate evidence.

<a id="snapshot-d864-product-93"></a>

## Historical d864: Historical capability matrix before this execution

The following matrix and dated recheck preserve the earlier candidate's evidence;
use the current receipt above for later source changes.

Source baseline: `b15a57f7c3a97c238b9361023753e0aed0ab74c5`, followed by the
focused local changes described below on the original PR28 branch. The
[current handover](README.md) records intake and recovery anchors.
The primary execution route is the authorized local Codex Desktop Mac;
ChatGPT + GitHub is auxiliary. Native/GUI/install work is no longer blocked by
the absence of an independent host. Specific safety refusals and resource
qualification remain separate from this host authorization.

| Requirement / real entry | Existing implementation retained | This batch / minimum next verification / exact limit |
| --- | --- | --- |
| Main conversation / `DocumentWorkspacePanel` | Core-owned proposals, real Diff, CAS, receipts and recovery; native quick actions; R07 lifecycle fixes | Removed three unused Write routes below. Full same-thread installed journey remains unverified. |
| DOCX / `office-worker.js` | Generation and native target selection | `target.setString` remains a fidelity risk, not proof every file is damaged. Reproduce mixed runs, links, fields, table paragraphs, repeated text and cross-run selection with the fixed engine; compare structure and visuals after reopen. Native execution still needs applicable refusal resolution. |
| XLSX / `officegeneration/workbook.go` | Typed number/formula/range operations already exist; Excelize 2.11.0 | Real pivot schema/handler and native reopen/recalculation are unfinished. Do not redo typed cells or substitute SUMIF text for a pivot. |
| PPTX / native worker and protocol | Styled generation and single-shape text replacement | Typed style/geometry/chart mutation with stable IDs/base revision and native preservation evidence remains unfinished. |
| Canvas / AtlasFlow and Core object editing | Existing scene, selection, notes, versioned operations, recovery and same-thread dispatch retained | Do not rebuild Canvas or reapply R07. PNG/JPEG region notes need original-coordinate/versioned anchors, persistence and same-thread references; numeric crop/rotate/mark is insufficient. |
| Media / `mediaexecution/service.go` | Fail-closed admission before non-generate media | image.edit still requires trusted local source-byte projection, pixel/metadata privacy, masks and Registry/Provider authorization; keep `ErrPrivacyUnavailable`. |
| Browser / `DevBrowserPanel` | Existing browser capability and lifecycle | Main-thread selection references need navigation/frame/revision identity and text/region anchors; stale/iframe/history cases and Chromium refusal resolution remain. |
| PDF / file / knowledge | Existing PDF text/page/rect reference, viewport, search/zoom and file surfaces | Preserve these owners; no reproduced rotation bug. Full shared synthetic journey not run. |
| Skills / installed package host | Installed snapshots, activation and generation binding | Upgrade/revoke/restart/in-flight callback and actual installed-copy discovery/body-loading acceptance remains. No new tool authority follows from reading skill text. |
| Write / editors and retrieval | Presets, quick actions, retrieval, completion, autosave, conflict review, undo/redo, export snapshots and save barriers retained | Three dead routes removed; cache invalidation races fixed, empty queries avoid scans, retained indexes and active builds are bounded. Production lifecycle invalidation/active release and runtime inline completion's temporary thread need further work; this does not complete the migration. |
| Private installation | Existing darwin-arm64 non-publish wrapper and source-bound resource admission | Old `8365b16ce` DMG is historical. Current-candidate installed native/IME/reopen/Provider evidence absent; SecurityAgent refusal not cleared by Mac authorization. |
| Integration / security | Original real history preserved | Fresh remote at 2026-09-21 02:42 UTC: PR28 open/draft at dde, main ce96, b15 not in main, b15 remote runs zero. dde CodeQL 37 high and two unresolved path review threads are not closed. No exact-candidate/main CI acceptance. |

<a id="snapshot-d864-product-121"></a>

### Historical d864: Completion recheck and bounded retrieval — 2026-09-20 20:06 PDT

**The adopted A—M execution request is not complete.** Its work packages remain:

| Package | Completion assessment | Unfinished work versus actual blocker |
| --- | --- | --- |
| P0 baseline/admission/security | Partial | ZIP/extracted bytes and latest local candidate verified; actual main/PR/rules/checks/reviews refreshed. Full CodeQL traces remain unavailable through the recorded connector endpoints; Rust root cause is not repaired. |
| P1 DOCX/reliable file journey | Not complete | Fixed-engine fidelity reproduction, repair based on that result, and actual proposal/Diff/accept/save/reopen journey remain. Applicable Chromium/SecurityAgent refusal resolution is missing; generic local Mac authority is already available. |
| P2 image/browser references | Not complete | Versioned region notes and browser selection production chains still require implementation and acceptance. Native/browser acceptance has a specific policy blocker; this does not block every source change. |
| P3 pivot/PPT native editing | Not complete | Real pivot schema/handler and PPT typed style/geometry/chart changes remain. Static implementation and native acceptance are distinct; do not describe all missing code as environment-blocked. |
| P4 lifecycle/cleanup | Partial | Three requested cleanup chains are source-verified. Cache invalidation, empty-query avoidance and aggregate bounds are implemented. Workspace/permission lifecycle release, same-thread completion and installed Skills lifecycle acceptance remain. |
| P5 installation/integration | Not complete | No current-candidate installed acceptance, push, Ready, merge or main acceptance. Specific outbound/native safety refusals are unresolved; candidate CI/product acceptance also remain. |

At 2026-09-21 03:01:52 UTC, actual main remains `ce96cf12581acfa0e19fae7c6aa9c709371012c8`,
PR28 is OPEN/Draft/unmerged at `dde784437dc8563e84066629dd57f4a11fd9acc9`.
The full returned check inventories are 64/64 for main (62 success, Rust and
Development gate failed) and 56/56 for dde (55 success, CodeQL failed with a
37-high summary). Two filestore review threads remain unresolved. Local start
candidate `33510c813fc3a93bb98c82ad746476bb9399c987` has zero remote workflow runs.
These inventories include different workflows; their totals are not a count of
required gates. The active strict Development ruleset still has no bypass actor.

The recheck reproduces two remaining cache resource defects: old workspaces
remain cached without a total cap, and new workspace requests can start unlimited
concurrent index builds. The existing cache owner now retains at most **eight
indexes**, evicts the least recently used on insertion, and prunes expired entries
on lookup. It permits at most **four active builds**, counting invalidated builds
until outstanding reads settle. A same-key request joins its admitted build;
capacity saturation returns no optional retrieval context without queueing more
scans, and later requests can succeed. Existing per-index file/chunk budgets,
clear-generation and promise-identity fences remain. These are owner resource
bounds, not a measured process-RSS guarantee, eager TTL expiry or complete
production workspace/revoke disposal. No timer, new authority or dependency was added.

Fresh evidence on `33510c813` plus this change, on the authorized local Mac with
`source ./scripts/use-analytix-cache.sh` in each shell:

- Original production code with three added cache tests: **2 fail / 9 pass**,
  exit 1. Both capacity regressions failed; normal cache hit/TTL refresh passed.
- Final `npm test -- src/main/services/write-retrieval-service.test.ts src/main/services/write-inline-completion-service.test.ts`:
  **2 files / 28 pass**, exit 0. Enabled/disabled retrieval now runs against a
  synthetic canary file and fake runtime: enabled reads that file and supplies
  the canary in runtime intent; disabled performs zero scans/opens and omits it.
  This verifies the Main-to-runtime intent boundary, not real Go privacy
  projection or final Provider traffic.
- `./node_modules/.bin/tsc --noEmit -p tsconfig.node.json` and focused ESLint on
  these three files: pass, exit 0. `git diff --check`: pass.
- Previous renderer/build evidence below is inherited for unchanged surfaces;
  no fresh full build, native engine, GUI, installation or real Provider run.

Five independent judgments remain: **main merged: no; main verified: no;
complete product acceptance: no; Formal RC: no; public publication: no**.
No third-party code was adopted. Remaining source work is not all blocked by
remote writes or native acceptance, and these source checks are not product completion.

<a id="snapshot-d864-product-176"></a>

### Historical d864: Previous focused local changes and evidence — candidate 33510c813

Retrieval: an invalidation generation prevents a late build from delivering or
recaching old snippets. An old completion can no longer remove a newer in-flight
entry for the same key. Scanning/reads check invalidation at asynchronous
boundaries; empty queries return before directory scanning. This is an in-memory
cache repair, not proof of production revoke wiring, disk indexing or disclosure.
The clear function still needs an appropriate production lifecycle owner.

Cleanup was checked against b15 source, dynamic imports and remaining consumers:

| Removed entry / caller evidence | Replacement owner | Retained compatibility and tests |
| --- | --- | --- |
| Unmounted `WriteAssistantPanelIsland` → lazy loader → `WriteAssistantPanel`, plus Workbench preload and its union/case | Main conversation and document callback | Shared `write-assistant-panel` CSS still serves `SubagentInspectorPanel`; SDD/i18n retained. Build verifies lazy/import closure. |
| No production caller for `ensureWriteThreadForWorkspace`, `createWriteThread`, `selectWriteThread`; associated state declarations and exclusive helpers/imports | Existing main-thread navigation | `openWrite`, registry hydrate/read/save/prune/forget, old title identification, archived history and route alias retained. Registry tests now construct legacy records directly instead of using retired creation helpers. |
| Sole `WriteWorkspaceView` caller always supplies `onSubmitPrompt`; optional direct-rewrite branch unreachable | Required same-conversation callback | Save/snapshot/read-only/selection/pending-Diff checks retained; Markdown/Rich/SDD inline completion, editor undo/review and recent edits retained. New component tests exercise callback, save failure and snapshot drift. |

Fresh local source evidence (all commands source `./scripts/use-analytix-cache.sh`
in the same shell; Node 22.22.1/npm 10.9.4, configured macOS):

- Retrieval's four new regressions: original production code **4 fail / 4 pass**;
  fixed code plus inline completion service: **24 pass**, exit 0, via
  `npm test -- src/main/services/write-retrieval-service.test.ts src/main/services/write-inline-completion-service.test.ts`.
- Cleanup: `DocumentWorkspacePanel`, registry, navigation, thread and side-action
  suites **151 pass**; new `WriteWorkspaceView.test.tsx` **10 pass** after fixing
  its incomplete i18n mock. Initial combined run had one suite initialization
  failure, not a product assertion failure. No assertion was weakened.
- `npm run typecheck`: both web and node configurations pass, exit 0.
- Preservation checks: `Workbench.canvas-send.test.tsx`,
  `workbench-document-message.test.ts`, `write-shutdown.test.ts`,
  `write-export-snapshot.test.ts`, `write-workspace-store.test.ts`: **75 pass**,
  exit 0. This covers existing R07/send, save/recovery, export and IME-barrier
  contracts, not real IME interaction.
- `npm run build`: pass, exit 0, including `build:runtime` and Electron
  main/preload/renderer. `npm run smoke:source`: pass, exit 0. Focused ESLint
  on retrieval, view, navigation, registry and corresponding new tests: pass.
- Total across the final successful, non-overlapping focused suites:
  **13 files / 260 tests passed**. `git diff --check` passes. These source
  checks do not establish native/GUI/installation acceptance.

No upstream source, prompt, asset or dependency was imported by this batch.
The cache owner/generation repair and cleanup are independent changes to the
existing Analytix implementation. Existing pinned upstream research remains
reference material, not proof of license admission, parity or native fidelity.
Keep restricted anthropics Office skill material out of the product.

<a id="snapshot-d864-product-222"></a>

### Historical d864: Historical capability matrix — c7 takeover, 2026-09-15

The following matrix and dated checks describe their original candidates;
they do not override the refreshed matrix above. In particular typed XLSX and
Canvas/R07 work must not be restarted from this older gap list.

Baseline: PR 28, `c7e66a7ba427cdd01eef1fe91aacd96ee18328c7`, on
`codex/workbench-product-delivery-20260914`. At takeover, Development gate passed,
but the independent CodeQL check failed with 12 path-injection alerts. Individual
successful language-analysis jobs do not close that check. Local changes listed
below are not yet installed-application evidence.

| Required outcome | As built / current work | Remaining evidence or implementation |
| --- | --- | --- |
| Unified workspace | Main conversation, object/tool tabs and separate terminal layout exist | Full keyboard, IME, narrow/wide, theme, scale and native geometry journey |
| Explicit native quick task | Same-thread callback carries its own frozen reference; shared action strip and owner-native context/keyboard menu, with preview-to-proposal dispatch | Real Provider/GUI journey and selection-positioned surface interaction |
| Quote-only send | Native references count toward composer send eligibility; quoting does not dispatch | Real Enter/button/IME journey and full action-entry integration |
| Annotation lifecycle | Per-object/thread notes persist through Core CAS; Main retains pending input and uses a renderer freeze acknowledgement before close/quit flush | Complete display anchors and multiple annotations; real normal-restart, capacity and closed-tab GUI journey |
| Native proposal refresh | Single-flight polling with failure backoff; new events supersede pending reads | Tool-completion refresh integration and installed behavior |
| DOCX | Core absent-only generation calls a data-only `docx` codec; checkpoint binds the created bytes and installation principal; opaque artifacts resolve into the current native workspace | Full headers/fields/links coverage, installed repeated-text positive case and representative rendering |
| XLSX | Go data-only generation writes typed cells, bounded checked formulas, formatting and native charts; sheet/chart IDs persist in OOXML; unsafe numeric/formula-to-text mutation remains rejected | Native recalculation/rendering, typed modification/range operations and summaries/pivots |
| PPTX | Structured generation writes text, shapes, native charts and validated images; stable slide/object IDs; native preview and bounded shape selection | Targeted style/chart edits, native ID roundtrip and per-slide visual quality |
| Canvas / images | AtlasFlow and existing local media assets remain reusable | Stable scene identity, fact/layout separation, selection/notes/local edits, versioned exports; real media Provider integration |
| Plugin Skills | All three Office 0.2.0 packages declare original workflows, fixed generation capabilities and installed snapshots; discovery and generation follow their signed activation | Canvas contribution, remaining dependency handlers and installed end-to-end evidence |
| Write migration | Existing MD/TXT editors, exports and same-thread document surface retained | Map all custom actions, presets, retrieval, autosave, conflicts, review and export to the shared product flow |
| Browser / files / PDF / knowledge | Existing surfaces remain | Shared authorized object opening and typed annotation anchors; no implicit indexing or uploading |
| Apply / recovery / Diff | Core-owned originals, exact save candidates and approved changes; thread-bound discovery, protected-local Diff, explicit interrupted-save continuation, restart undo and cancellation of proven unsubmitted changes | Native format fidelity and actual third-object/restart GUI journey |
| Installation | Source `8365b16ce` produced a private macOS ARM64 DMG; container/layout/signature checks and isolated installation signature checks passed | First launch blocked before application initialization by Chromium default-Keychain authorization; installed end-to-end journey and separate public redistribution obligations remain |
| Security / delivery | Existing macro prohibition, privacy authority and protected profiles preserved; controlled object reads/commits reject hard-link aliases, including post-inspection drift | Exact-candidate CodeQL, applicable CI/review and remaining required negative cases |

The numeric/formula text-path restriction is interim damage prevention. It is not
acceptance of a text-only spreadsheet product. Persisted note text alone is not
a complete position-aware annotation system. Source tests are not native engine fidelity,
live media generation, or installed GUI evidence.

The initial selection repair was checked with six focused Vitest files (86 tests)
and `tsc --noEmit` for both `tsconfig.web.json` and `tsconfig.node.json` on the
configured macOS host, using `scripts/use-analytix-cache.sh`. The quick-dispatch
and note-retention regressions failed before their fixes; numeric/formula coercion
also failed for both native types before the worker guard. Deferred-response tests
cover version change, superseding events, collapse/remount polling and scope
revocation after changing tabs. This is synthetic source-level evidence only.

The native menu checks cover owner/main-frame validation, target-version changes,
opening without dispatch, selection changes while open, existing non-ASCII custom
action IDs, and one-click preview-to-proposal capture. The renderer suite passed
27 tests separately; Main IPC/surface suites passed 27 tests. Mixed-environment
runs encountered a Vitest worker-start timeout, not a passing combined run.
Hard-link regressions failed before the repair and passed after integration;
ordinary text-tool hard-link compatibility remains covered. Independent review
accepted these bounded source changes. Cross-compilation is not Windows runtime
evidence, and no CodeQL closure is claimed from the local filesystem tests.

The DOCX generation candidate uses a fixed host-supplied codec executable and
entry, bounded stdin/stdout and no inherited credential environment. The codec
receives content and explicit image bytes, never the target workspace path.
Core validates the OOXML package, creates only an absent target and settles the
checkpoint before issuing typed artifact metadata. A configured bundle is still
required; this is not a claim of packaged admission. Hosted lifecycle source checks
are recorded separately below.

Generation tool arguments retain the existing execution-grant limit of 4 MiB
JSON and 1 MiB per UTF-8 string. Generation-specific operation records now support
that limit through protected CAS writes, readback and restart validation. Ordinary
text snapshot and mutation limits remain unchanged. The codec's larger private
asset limit does not expand the model tool argument limit. Large source images
still need a Core-authorized asset-reference path.

Artifact opening validates the current installation principal, conversation,
workspace, root identity, file links, content hash and package structure. New
live receipts open the native object through the same resolver as the artifact
card; replaying an existing receipt does not automatically reopen it. Switching
conversation/workspace or a failed pending text save cancels the open. The public
receipt contains no file path, raw document content or evidence authority.

Current checks include 245 tests across eight focused Vitest files (including
51 DOCX codec/builder tests), both TypeScript configurations, a real
Go-to-bundled-codec integration producing and inspecting Chinese headings/lists/
tables, and focused Go generation/checkpoint/authority tests.
Independent review found two artifact timestamp/digest privacy seams, at durable
append and subsequent public projection. Both now reuse one strict closed-host
metadata predicate. The regression exercises real settlement, durable append,
HTTP SSE replay and replay after reopening the store; ordinary PII, malformed
lookalikes and other tools retain their existing rejection/projection behavior.
The production-tag Go runtime also compiles, and the repository's Electron Main
build emits the fixed codec entry and its chunks into a task-owned cache layout.
The Go integration also passes against that emitted JavaScript entry, producing
and inspecting a real DOCX through the production codec process boundary.
That layout uses the current checkout's dependency tree and is not an independent
installation or a distribution artifact.
No native GUI rendering or installed journey has been established by these checks.

Binary generation settlement and startup reconciliation are distinct from
checkpoint deletion/restore. The existing generic checkpoint rescue is text-only;
generated Office creation therefore reports `manual_review`, not a false
`ready/delete_created_file`. This does not disable native modification proposals
or native undo. Native edit/undo persistence remains an active required outcome.

Documents lifecycle checks use the actual repository package, a task-isolated
installation authority and installed copy. They pass for fixed-byte reading,
source/installed separation, installed tampering rejection, current discovery,
inline instruction loading, generation preparation, disable/re-enable rejection
and Host restart with the same snapshot. Host, catalog and side-effect preparation
packages pass their tests; independent source review found no blocker in this
bounded integration. The Skill validator and diff check pass. These are real
materialization fixtures, not an installed desktop GUI journey.

<a id="snapshot-d864-product-329"></a>

## Historical d864: Three-format generation integration

The shared `generate_office_document` request now discriminates DOCX Markdown,
XLSX typed workbook data and PPTX structured slides. The model-facing schema only
advertises a kind while its exact installed Skill is enabled and its format
writer is available. A project Skill cannot impersonate any of the three fixed
Office namespaces. Host activation, source identity and Skill digest remain part
of the prepared operation; all formats use the same Core create/checkpoint,
opaque receipt and protected-local opening authority.

XLSX uses pinned Excelize in Go without an additional Python or Node dependency.
It keeps literal `=` strings as text, finite numbers and booleans typed, and
formula expressions intact. The finite formula subset has bounded references,
dependency depth, cycle checks and real calculator checks. SUMIF requires static,
equally shaped ranges to prevent unvalidated implicit expansion. Sheet identity
uses Unicode simple folding consistently with the writer. Charts reference an
existing text-label cell and bounded category/value vectors on that sheet.
Formulas request native recalculation; persisted typed caches are not fabricated.

PPTX uses pinned PptxGenJS through the same fixed data-only process entry as DOCX.
Coordinates are bounded to a 16:9 page; IDs are globally unique. Object names use
the writer's public API; each validated slide ID is written into its standard
`p:cSld/@name` attribute in newly generated bytes. Images are decoded and checked,
including repeated-embedding budgets. No URL, template path or file API is
available in the admitted input. A missing optional title remains valid.

Current integration checks pass for three-format current discovery, inline
instruction loading, prepared generation and disable/re-enable behavior, and
for artifact settlement, durable append and SSE replay before/after store reopen.
The emitted Electron Main entry and Go composite produce actual DOCX, XLSX and
PPTX from the same synthetic 100/200/300 data and pass Core OOXML inspection;
this also verifies XLSX generation without Node. Format/builder tests pass
101/101, artifact IPC/opening tests 16/16, both TypeScript configurations pass,
and both new Skill validators pass. The production-tag Go runtime builds.
These are bounded source/runtime checks, not installed GUI evidence.

The format dependencies and exact notice texts are recorded in
[office-generation-dependencies.md](../office-generation-dependencies.md). They do
not establish native engine/font admission, packaged dependency presence or GUI
acceptance. Independent review found and fixed the missing-title wrapper,
noncanonical PPTX field names, SUMIF range expansion and Unicode sheet collision.
The SUMIF regression was observed failing before the fix. The corresponding
formula/alias calculator checks and format tests pass; visual/native roundtrip
and complete installed product acceptance remain required.

<a id="snapshot-d864-product-374"></a>

## Historical d864: Native review and recovery integration

Core captures original bytes before releasing an approved replacement. Private
records bind the installation principal's object identity, conversation, proposal,
revision and fixed save/undo operations. Fresh sessions can discover the current
change without Main remembering an operation ID. Both a reopened commit and undo
participate in the shared managed-file capture and binary CAS. Local review shows
restored original values separately from model-facing protected parts.

Prepare reservations and a bounded retiring list make interrupted metadata/large
blob cleanup discoverable. Superseded/cancelled changes retain compact replay
records; large originals are emptied through precise CAS after retirement.
Unresolved undo originals are retained. Cancellation applies only to confirmed
unsubmitted changes; existing save journals, including ambiguous conflicts, are
not treated as proof that no write occurred. A durable undo intent with no undo
journal can be explicitly continued after restart.

Fresh verification passes for both Go application packages and the focused
filestore Office/object-editing/recovery suite. A real three-format file fixture
passes approval, save, fresh Store/Service/Adapter discovery, undo, replay and
external-version rejection, with managed capture asserted during each CAS.
Main/controller/IPC/contracts pass 64 tests; the renderer's 30 tests cover local
Diff, missing-review refusal and thread isolation. Both TypeScript checks pass;
focused ESLint has zero errors and three existing effect-dependency warnings.
Independent storage review found no additional concrete defect in that candidate.

Core now retains the exact exported candidate, bounded to 16 MiB, before the first
save journal. A fresh session can explicitly continue that approved change using
only its identity and original revision. Core checks private file identity, hash,
OOXML kind, thread ownership and current original bytes before the same-operation
CAS. A completed journal remains query-only. Ordinary commit retries and status
queries never resume a pending write, including when the journal is absent.
Legacy records without the retained candidate cannot gain resume permission from
caller-supplied bytes. Retired originals and candidates are emptied through CAS.

The desktop exposes this continuation separately from result checking. A failed
query refreshes the Core recovery capabilities without writing; a new explicit
click is required to continue. After a lost reply, fixed-operation result checking
and exact current-byte validation precede native reload. This preserves uncertainty
without permanently locking the Main controller after a pre-journal interruption.
Native GUI/format fidelity and installed recovery remain unverified by these
source/fixture checks. Note persistence is covered separately below.

The explicit-continuation candidate passes the two Go application suites and
the focused filestore native recovery/resume and object-editing tests (8.900s).
The three desktop/controller/contract files pass 75 tests, including query-only
capability refresh, fresh-controller continuation and lost replies. Both
TypeScript configurations pass. Independent review caught and verified the fix
for ordinary replay with a missing journal; that path now returns unknown without
creating a journal or candidate. These checks use synthetic files on the configured
macOS host and do not establish installed recovery or native formatting quality.

<a id="snapshot-d864-product-426"></a>

## Historical d864: Annotation note persistence and safe exit

Core stores one note draft per object identity and conversation with a revision
CAS, protected file permissions and exact failed-request replay. Main retains
new input synchronously while an earlier save is pending; a stale acknowledgement
cannot replace later text. Normal object close, capacity eviction, thread change
and application exit flush pending notes. A failed save keeps the note and offers
retry. Restored text and its historical source revision do not recreate an old
selection token or grant modification authority.

A renderer/preload acknowledgement freezes note input before the final close or
quit flush. Independent holds prevent concurrent file close and application quit
from unfreezing each other. A missing acknowledgement leaves input state unknown:
legal notes remain receivable, while the next close must obtain a fresh freeze
acknowledgement. Core and PTYs stop only after exit is accepted; cancelling exit
or hiding to the tray preserves the running session.

Focused Main, IPC, actual before-quit callback, preload and renderer tests pass
75/75. Core annotation/recovery filestore and the three relevant application
packages pass. The old-acknowledgement overwrite regression failed before repair.
Independent read-only review accepted the bounded close/quit corrections. These
checks establish note persistence and shutdown ordering in source/fixtures;
multiple location-aware annotations, installed IME behavior and normal-restart
GUI acceptance remain required.

<a id="snapshot-d864-product-451"></a>

## Historical d864: Qualified private-local installation candidate

A separate build entry stages the pinned engine, surface/preload, Office packages
and notices before the existing packaged authority and resource seal. Core derives
the directory from its real executable inspection, refuses insecure private
composition and never falls back to development roots when package verification
fails. Current resource identity is checked before and after materialization and
Host operations. The server retains the concrete adapter pointers for binding
selection/privacy/capture; the Host wraps those same instances for qualification.

Main uses a nonce-bound, bearer-protected local-display query unavailable to the
Renderer. A private internal token permits loading only the fixed qualified
resource directory. The metadata digest and held bytes must agree with Core,
including the native preload. Normal source startup retains its original gate.
Note input is handed to an existing Main controller synchronously so a new
qualification query cannot delay the last keystroke past a close acknowledgement.

The independent codec bundle includes its JavaScript dependencies and leaves only
Node builtins external. Its exact two-file directory is unpacked and checked before
packaged authority generation. Go through a real Electron Helper successfully
generates and inspects all three formats from an isolated copy; the bundle does
not resolve the repository dependency tree. This proves that bounded execution
chain, not an actual installed application or native visual result.

Current checks: 99 focused Main/native/admission/annotation/quit tests and all
56 process-launch tests pass. Fourteen qualification/staging checks and four
private build-scope checks pass. Go asset/materialization packages, protected
admission routing and production-tag composition checks pass. Independent review
caught and corrected a missing transport allowlist, insecure authorization,
post-operation qualification and a concrete-adapter binding regression. Main's
qualification metadata also cross-checks the production JavaScript contract.
The original source gate, signed activation and file/privacy authority remain.
The installation probe below supplies package staging and signed-payload evidence
for its exact source. Native GUI and complete installation acceptance remain;
no public-release permission is established.

<a id="snapshot-d864-product-487"></a>

## Historical d864: Private installation probe, 2026-09-15

Source `8365b16ce42b143e4163931fd944222d6d71995d` built with
`scripts/package-office-private-local.mjs`, using the fixed host Office assets
and a fresh output directory. All four development native components, desktop
bundles, packaged Go runtime, private Office qualification, staged payload seal,
application signing and DMG creation completed. The resulting
`analytix-1.0.6-mac-arm64.dmg` is 469609175 bytes, SHA-256
`a25295033de0527a50d48f8e6d9862ef73bb45b54aed2f87508e885e5344e508`.
Its classification is `development_clean_non_publishable`; `publishable` and
`releaseEligible` remain false. No artifact was published.

`node scripts/package-candidate-smoke.mjs <fresh-output-directory>` passed DMG
verification, read-only mounting, product layout, bundle identity and strict deep
signature verification. The app was then copied from that read-only DMG to an
exclusive task installation directory. Its signature passed again, and the
installed qualification and package authority reference the same source commit.
This does not establish notarization, upgrade, native rendering or product RC.

A new named synthetic-only profile was created on the managed local filesystem,
with separate application/runtime state, an explicit task Keychain and a retained
controller whose reconnect endpoint was checked before provisioning. No prior
profile or credential was copied. Previously generated synthetic Office files
were copied only as opening fixtures; they are not installed-UI generation
evidence. The separate loopback-only synthetic Provider passed metadata health
checks but was not configured through the app or used for a model task.

The first installed launch stalled before application initialization. A bounded
sample showed `SecItemAdd → defaultKeychainUI → AuthorizationCopyRights` in
Electron/Chromium. The Go task-Keychain binding does not bind that cookie-encryption
path. Computer Use refused access to `com.apple.SecurityAgent`. The task-owned
launch was stopped; the installation, profile, original task Keychain and evidence
were retained. No default-Keychain policy or encryption setting was changed.
An independent usable macOS acceptance session is requested for this dependent
GUI path. Real text/media Provider authority also remains unavailable. Other
implementation and review continue; none of these gaps reduces the completion
contract below.

<a id="snapshot-d864-product-525"></a>

## Historical d864: Completion evidence

Use synthetic materials in one conversation to generate XLSX, then DOCX and PPTX
from the same data, then a structured canvas and image. Select native content,
dispatch explicit tasks, review and apply changes, verify untouched objects and
types, undo, close/reopen, switch tabs and restart the installed application.
Inspect real package structure, formula expressions/results, styles, object
identity and representative rendering in addition to screenshots and hashes.

Cover stale thread/workspace/object/revision/generation, incomplete captures,
hostile embedded instructions, external modification, duplicate apply, lost
commit replies and same-operation recovery, plugin disable/upgrade, third-object
eviction, annotation drafts, macros/external relations and protected model
projection versus authorized local display. Unresolved data loss, type corruption,
wrong-range modification, privacy leaks or false saved status prevent acceptance.

Delivery includes the exact source/PR/check state, installer path/hash/version and
admission status, actual sample files before/after changes, self-contained GUI
evidence, plugin/Skill capability and dependency inventory, and the Write migration
map. Missing real media authorization, engine distribution obligations, signing
or an unavoidable system interaction blocks only the dependent evidence. It does
not remove that requirement or establish overall completion.
