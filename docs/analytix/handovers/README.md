# Analytix 施工交接索引

Status: Operational index
Applies to: 跨线程暂停、恢复与阶段性交接
Source of truth: 当前代码、当前适用 `openspec/changes/*/tasks.md`、当前 Git/GitHub、当前 PR/CI/review 和确有必要的新验证
Supersedes: 无；本目录不替代 accepted specs、OpenSpec 或代码

本目录只保存“某一时点如何安全接续施工”的操作性快照。交接文档可以说明当时
看到了什么、运行过什么、还缺什么，但不能把历史 PASS 自动继承给新的工作区。
**本页是跨线程恢复的恒定入口**：不要为每个新会话另建第二套总账。

## 当前接续：2026-09-17 Canvas 受控写入修复

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

## 最新保留快照

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

## Historical

- [`2026-08-04-controlled-thread-checkpoint.md`](2026-08-04-controlled-thread-checkpoint.md)：
  当时的 canonical 收敛、缓存容量与 Milestone A/B gap；其中旧镜像保留和容量
  blocker 状态已由 2026-08-05 checkpoint 取代。
- [`2026-07-25-thread-close-route-audit.md`](2026-07-25-thread-close-route-audit.md)：
  当时的产品路线复核、Codex/Claude Code/OpenCode 对照、Obsidian 审核和
  上游 currentness；不替代当前知识治理或 2026-09-16 checkpoint。
- [`2026-07-25-first-stage-pause.md`](2026-07-25-first-stage-pause.md)：
  第一阶段施工暂停时的工作区、Goal、P0-P4、资金插件、上游吸收、验证和风险
  快照；仅作 Historical 背景。

## 新线程恢复顺序

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

## 交接最小合同

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

## 交接状态词

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

## 单一事实链

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

聊天记录可以帮助解释，但不是必需依赖。即使更换会话、Notion 临时不可用或旧线程
无法访问，只要 GitHub 可读，新线程仍应能从本页和最新 checkpoint 恢复到可施工状态。
