# Analytix 施工交接索引

Status: Operational。唯一恢复入口；以下是 2026-09-29 PDT 的接续快照，
恢复时重读 Git 与适用验证，不把此页当作实时锁或测试回执。

Repository `Eysn0130/analytix`；canonical `/Users/sun/Projects/analytix`；
branch `codex/workbench-product-delivery-20260914`；[PR28](https://github.com/Eysn0130/analytix/pull/28)。

## 当前恢复点 — 2026-09-29 PDT

| 项目 | 当前观察与边界 |
| --- | --- |
| 继承 HEAD / tree | `c2cd3e29174e701569220e1c39076e0acff24a53` / `55893ead1fc3a6bce73ede22a251005a14810532`；延迟 bundled Funds activation。入口整理已在 `39250cafa` 本机提交；实时 HEAD 以 Git 为准。 |
| writer | 本轮唯一主施工线程 `01a0f08d-4ef4-7f82-a30b-8a94c014ac26`，保存的 local analytix 项目，实际 `gpt-6.1-sol / max`。协调桥只读。其他历史 worktree 不在本轮写入范围；不能把残留锁或进程等同于 active writer。 |
| 继承 dirty | 开始时 34 tracked 修改、26 untracked 文件、0 staged；主要为 B1 host-local，另有用户 runbook 和 QA 草稿。保留原字节，按 hunk 来源审查，不 reset/clean/stash/跨分支迁移。该数量是开始快照，后续施工会增加任务自有修改。 |
| 最新安装证据 | 精确 SOURCE `7dc07bb0973e2c5e6a5e5c9235bd585c9c3a1495`，两轮隔离合成 K10 正常退出、零残留；`032b8a147` 是 observer 修正，产品字节未改变。见[准确诊断](../qa/pr28-n03-7dc07bb-installed-diagnostic-2026-09-27.md)。不转移到 c2cd/B1。 |
| 当前 B1 | [host-local source checkpoint](../qa/pr28-b1-host-local-currentness-source-2026-09-27.md)：empty-lineage mode、DSV2、Registry CAS 与部分重启已有未提交候选；Final/recovery、清洗前驱、CAS loser 认证恢复及安装完整旅程仍缺。N06/B16 开放，整 profile 回滚 `UNVERIFIED`。 |
| 远端 / main | 本轮尚未 fresh 取得 PR/head/main/CI。旧 56231 的 CI 不覆盖较新本机源码；Draft/open 是前序记录，当前状态必须独立读取。 |

## 下一依赖有效动作

1. 本轮已在 `39250cafa` 修薄入口，并运行 `node scripts/validation-burden.mjs`。报告只做
   stdlib 只读盘点，基线与 matched read 范围见[小 manifest](../validation-burden-baseline.json)；
   它不选掉测试，不宣称 token、时间、正确率或分析收益已改善。
2. [投影保真源码检查点](../qa/pr28-account-flow-projection-fidelity-2026-09-29.md)
   已记录主体/对手方身份错配修复、独立金额真值与调用方验证；不等于分析收益。
   保留并核查 host-local 未提交合同与适用 OpenSpec；在隔离数据上补最小缺失
   行为和独立语义 oracle。所有安全/撤权/恢复检查针对实际稳定候选；普通
   Agent 不依赖 Funds 初始化。命令先按 [runbook](../development-runbook.md)
   同 shell 加载缓存 helper，任务资源隔离；不触碰既有用户 profile。
3. 同模型/数据/预算的分析 pilot 区分表示保真、端到端增益、隐私权限与安装
   可用性；只使用合成或已明确许可输入。费用与真实 Provider 权限未明时，
   先推进确定性 oracle，不用合成结果替代真实 Provider/安装验收。
4. 达到相应门槛后聚焦提交、正常源码同步、准确候选 CI/review/适用安装验收，
   再决定 merge/main。公开发布另行满足资格；当前没有完成结论。

长期方向与不变分母见[唯一执行方案](../delivery-execution-plan.md)，
能力、候选、证据与缺口见[产品矩阵](../product-completion.md)。
旧叙述已归入既有[日期 QA](../qa/pr28-next-execution-2026-09-26.md#historical-entry-snapshots-relocated-from-c2cd--2026-09-29-pdt)，原锚点保留如下。

## Historical entry anchors

<a id="当前恢复点--2026-09-27-pdt"></a>

- [当前恢复点 — 2026-09-27 PDT](../qa/pr28-next-execution-2026-09-26.md#snapshot-c2cd-handover-8)

<a id="c8-源码检查点保留"></a>

- [c8 源码检查点（保留）](../qa/pr28-next-execution-2026-09-26.md#snapshot-c2cd-handover-35)

<a id="e15-凭据与安装验收检查点保留"></a>

- [e15 凭据与安装验收检查点（保留）](../qa/pr28-next-execution-2026-09-26.md#snapshot-c2cd-handover-46)

<a id="历史恢复点--2026-09-24-pdt"></a>

- [历史恢复点 — 2026-09-24 PDT](../qa/pr28-next-execution-2026-09-26.md#snapshot-c2cd-handover-66)

<a id="historical-handover-entries"></a>

- [Historical handover entries](../qa/pr28-next-execution-2026-09-26.md#snapshot-c2cd-handover-97)

The following links preserve prior entry URLs; their targets are dated evidence, not current next actions.

<a id="当前接续pr28-an-新反例与固定引擎验证--2026-09-21-pdt"></a>

- [当前接续：PR28 A–N 新反例与固定引擎验证 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-12)

<a id="历史接续pr28-browser-与证据复核--2026-09-21-pdt"></a>

- [历史接续：PR28 Browser 与证据复核 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-46)

<a id="历史接续pr28-独立审查落地--2026-09-21-pdt"></a>

- [历史接续：PR28 独立审查落地 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-65)

<a id="历史接续pr28-双执行器--2026-09-21-pdt"></a>

- [历史接续：PR28 双执行器 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-79)

<a id="历史接续pr28-am-本地源码推进--2026-09-20-pdt"></a>

- [历史接续：PR28 A—M 本地源码推进 / 2026-09-20 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-87)

<a id="历史接续完整性复核与缓存资源上限--2026-09-20-pdt"></a>

- [历史接续：完整性复核与缓存资源上限 / 2026-09-20 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-109)

<a id="上轮本机接收与开发路线授权"></a>

- [上轮本机接收与开发路线授权](2026-09-16-pr28-continuation.md#snapshot-d864-handover-133)

<a id="历史接续r07-合法后续刷新生命周期--2026-09-18-utc"></a>

- [历史接续：R07 合法后续刷新生命周期 / 2026-09-18 UTC](2026-09-16-pr28-continuation.md#snapshot-d864-handover-160)

<a id="历史接续2026-09-17-回执归属修复--2026-09-18-utc"></a>

- [历史接续：2026-09-17 回执归属修复 / 2026-09-18 UTC](2026-09-16-pr28-continuation.md#snapshot-d864-handover-197)

<a id="历史接续2026-09-17-实际发送链与案件授权组合"></a>

- [历史接续：2026-09-17 实际发送链与案件授权组合](2026-09-16-pr28-continuation.md#snapshot-d864-handover-233)

<a id="历史本地候选2026-09-17-canvas-引用持久审阅与消费者闭环"></a>

- [历史本地候选：2026-09-17 Canvas 引用、持久审阅与消费者闭环](2026-09-16-pr28-continuation.md#snapshot-d864-handover-268)

<a id="历史检查点2026-09-17-独立构建环境与回执授权边界"></a>

- [历史检查点：2026-09-17 独立构建环境与回执授权边界](2026-09-16-pr28-continuation.md#snapshot-d864-handover-307)

<a id="已落实的构建依赖"></a>

- [已落实的构建依赖](2026-09-16-pr28-continuation.md#snapshot-d864-handover-317)

<a id="本批真实产品修复与验证"></a>

- [本批真实产品修复与验证](2026-09-16-pr28-continuation.md#snapshot-d864-handover-335)

<a id="ci安全和-duckdb-的真实边界"></a>

- [CI、安全和 DuckDB 的真实边界](2026-09-16-pr28-continuation.md#snapshot-d864-handover-360)

<a id="下一依赖有效动作"></a>

- [下一依赖有效动作](2026-09-16-pr28-continuation.md#snapshot-d864-handover-380)

<a id="历史检查点2026-09-17-canvas-受控写入修复"></a>

- [历史检查点：2026-09-17 Canvas 受控写入修复](2026-09-16-pr28-continuation.md#snapshot-d864-handover-394)

<a id="最新保留快照"></a>

- [最新保留快照](2026-09-16-pr28-continuation.md#snapshot-d864-handover-433)

<a id="historical"></a>

- [Historical](2026-09-16-pr28-continuation.md#snapshot-d864-handover-460)

<a id="新线程恢复顺序"></a>

- [新线程恢复顺序](2026-09-16-pr28-continuation.md#snapshot-d864-handover-472)

<a id="交接最小合同"></a>

- [交接最小合同](2026-09-16-pr28-continuation.md#snapshot-d864-handover-497)

<a id="交接状态词"></a>

- [交接状态词](2026-09-16-pr28-continuation.md#snapshot-d864-handover-515)

<a id="单一事实链"></a>

- [单一事实链](2026-09-16-pr28-continuation.md#snapshot-d864-handover-531)
