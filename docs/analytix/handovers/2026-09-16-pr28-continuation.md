# Analytix PR #28 跨线程接续检查点

Status: Operational checkpoint；不是产品验收或 release authorization
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
