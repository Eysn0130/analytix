# Analytix 施工交接索引

Status: Operational index
Applies to: 跨线程暂停、恢复与阶段性交接
Source of truth: 当前代码、当前 `openspec/changes/*/tasks.md`、当前 Git
工作区、仍适用的候选绑定证据和确有必要的新验证
Supersedes: 无；本目录不替代 accepted specs、OpenSpec 或代码

本目录只保存“某一时点如何安全接续施工”的操作性快照。交接文档可以说明当时
看到了什么、运行过什么、还缺什么，但不能把历史 PASS 自动继承给新的工作区。

## 最新保留快照

- [`2026-09-10-owner-replacement.md`](2026-09-10-owner-replacement.md)：
  用户授权新 Owner 替换、旧 AI 永久撤权、48 文件 R131 候选与 F1 保留、
  无自动化的 Slice 主动回传路径及下一项验收义务。不是 R131 或 A0/B1 PASS。
- [`2026-08-05-damaged-cache-retirement.md`](2026-08-05-damaged-cache-retirement.md)：
  当时 clean canonical 基线、损坏缓存退役、v3 保全与容量状态，以及当时
  OpenSpec、A0/B1 线程的前置条件。仅保留该日期的缓存和恢复历史，不是当前
  施工或验收 authority；当前任务必须按本页下方恢复顺序 fresh 重建。
- [`../document-consolidation-register.md`](../document-consolidation-register.md)：
  项目文档分类、碎片归并和历史文档治理登记。

## Historical

- [`2026-08-04-controlled-thread-checkpoint.md`](2026-08-04-controlled-thread-checkpoint.md)：
  当时的 canonical 收敛、缓存容量与 Milestone A/B gap；其中旧镜像保留和容量
  blocker 状态已由 2026-08-05 checkpoint 取代。
- [`2026-07-25-thread-close-route-audit.md`](2026-07-25-thread-close-route-audit.md)：
  当时的产品路线复核、Codex/Claude Code/OpenCode 对照、Obsidian 审核和
  上游 currentness；不替代 2026-08-04 checkpoint。
- [`2026-07-25-first-stage-pause.md`](2026-07-25-first-stage-pause.md)：
  第一阶段施工暂停时的工作区、Goal、P0-P4、资金插件、上游吸收、验证和风险
  快照；仅作 Historical 背景。

## 新线程恢复顺序

1. 读取根 `AGENTS.md`、`docs/analytix/README.md`、本页和最新一份交接。
2. 执行 `git status --short --branch`、`git rev-parse HEAD`、
   `git diff --stat` 和 `openspec list --json`。
3. 比对交接记录与当前工作区；任何差异都按新的事实处理，不覆盖用户修改。
4. 读取 active change 的实际 `tasks.md`，重新计算任务分母和完成数。
5. 对“上次命令仍在运行”“结果丢失”“未复跑”“外部环境缺失”分别标记，
   不把它们改写成 PASS。
6. 只为当前批准的施工切片建立验证计划；交接中的 backlog 不自动进入范围。

## 交接状态词

| 状态 | 含义 |
| --- | --- |
| `PASS` | 在记录的工作区和环境中实际执行并通过。 |
| `FAIL` | 实际执行且失败，结果仍有效。 |
| `ABORTED` | 人工终止或因范围冲突终止，不是产品失败证明。 |
| `RESULT LOST` | 命令曾运行，但无法取得最终退出状态或完整结果。 |
| `UNVERIFIED` | 没有满足真实环境、凭据、主机、数据或重新执行条件。 |
| `NOT IMPLEMENTED` | 代码路径或生产组合尚未存在。 |
| `BLOCKED` | 明确前置条件未满足，无法进行该项验证。 |

不得用文件存在、fixture 通过、进程退出码为零、历史报告标题含 `final` 或
active OpenSpec artifact 已生成来替代产品级验收。
