# Analytix 施工交接入口

Status: Operational。恢复入口只路由当前来源，不保存实时锁或继承 PASS。
canonical `/Users/sun/Projects/analytix`；当前 branch/HEAD/index/dirty 以 fresh Git 为准。

1. 读取当前请求与适用 `AGENTS.md`；核对 Git、保护字节和 writer/process 状态。
2. 使用 `node scripts/validation-burden.mjs --json` 获取派生 active OpenSpec 状态；
   只读取已授权 target 与真实依赖，不递归读取全部 tasks 或日期 QA。
3. 从 [执行方案](../delivery-execution-plan.md) 与 [产品矩阵](../product-completion.md)
   读取任务所需段落，核对实际消费者和 candidate-bound evidence，继续下一有效缺口。
4. 按 [runbook](../development-runbook.md) 的有关命令验证；同 shell source 缓存 helper。
   历史安装、CI、Provider、签名结果不转移到当前 dirty。

当前 commissioned Owner 的控制记录由该 Owner 唯一维护。交接页与历史 writer
标识不能授予 lease。候选完成后交付准确文件归属/哈希/命令/环境/结果与未验证范围。
Git 与外部操作仍遵守当前任务授权及 [workflow](../git-workflow.md)。

历史接续可通过下面兼容锚点按具体事实查询；不会默认打开其正文。
2026-09-29 接续快照及 B1 来源见 [保留 QA](../qa/pr28-next-execution-2026-09-26.md#historical-entry-snapshots-relocated-from-c2cd--2026-09-29-pdt)。

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
