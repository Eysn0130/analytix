# Analytix 施工交接索引

Status: Operational。唯一当前恢复入口；历史回执不授予权限，也不替代 accepted specs。

## 当前身份与恢复顺序

Repository `Eysn0130/analytix`；canonical `/Users/sun/Projects/analytix`；
branch `codex/workbench-product-delivery-20260914`；[Draft PR28](https://github.com/Eysn0130/analytix/pull/28)。
原 Codex 保持唯一 source writer，未交接。下一方默认 `REVIEW_ONLY`。

本轮 START `d8640957c4c1c69c5ddee4e3a9b017382c088047`，tree
`2e4576860898320de4805fd5090540c6386ce724`，接收时 clean/ahead61。
最新 SOURCE `2f25c22f7c8fb9626e204e34d573fb1de3e7208b`，tree
`63b1edccec3475abc7990776e77b5cda2df92245`；随后文档 DELIVERY 是包含本页的提交，
精确 SHA/tree/dirty 在交付 manifest 单列，不把文档提交借作重新运行源码的证据。
保留148d、fe656、65ab、5242、4343、d864及所有有效后继。

1. 先核对 canonical 实际 HEAD、tree、branch、dirty 与 writer，保护后继和用户修改。
2. 阅读适用 AGENTS、[文档地图](../README.md)、[规范登记](../specs/README.md)、
   当前授权范围及实际 owner。查看[当前产品矩阵](../product-completion.md)和
   [本轮 QA](../qa/pr28-independent-review-execution-2026-09-21.md#current-readiness-continuation-from-d864)。
3. 再 fresh 读取远端 PR/head/base/main/checks/review。上轮已观察 remote dde、main ce96，
   本地后继未同步，不能用旧远端重建或覆盖它。任何旧 SHA 的绿灯都不自动继承。
4. 从依赖已满足的缺口继续；未失效证据复用，不重跑全部测试、不重建 Canvas、不恢复第二路线。
5. canonical 事实提交后才镜像原 Notion 台账；不向镜像写源码、私有原始日志或个人数据。

## 本轮真实成果与剩余

固定 DOCX 字号来源审计和两个对照已执行：docDefaults 11pt、段落10.5pt/run11pt覆盖
均在固定引擎导出重开稳定。原样本无字号声明时，原有11/10.5→11/11损失记录保留。
SOURCE新增正文/表格字号检查，在编辑准入和导出前拒绝已知中西文字号不一致；
保留原件、只读预览及普通/跨run正向编辑，未把输入归一化成11pt。
这是已知损失保护，不是完整格式保真或 installed 产品验收。

| 剩余项 | 类型 | 下一最小动作 |
| --- | --- | --- |
| DOCX非目标内容、关系、页眉/脚注/视觉完整保真及Core文件闭环 | EVIDENCE_NOT_RUN / SOURCE_GAP | 在现有worker/codec/Core CAS链验证候选；保护不能等同全文件已验收 |
| 导入pivot/chart局部编辑 | SOURCE_GAP | 稳定part/relationship/field身份及固定UNO typed事务；shared-cache、冲突、重开 |
| Skills远程archive安装、升级、卸载 | SOURCE_GAP | 在原materialization owner接入可核验publisher/registry授权及stage/journal/revoke；hash不自授权 |
| Chromium任务隔离存储 | SOURCE_GAP / ENVIRONMENT_LIMIT | 固定ElectronOSCrypt在Core前的合法存储边界；不关Cookie加密、不改全局Keychain搜索表 |
| Browser/图片/Skills共同安装旅程与真实Provider | EVIDENCE_NOT_RUN | 准入后准确候选安装及正常Registry/Secret Store；不借组件测试作GUI通过 |
| 当前67告警调用链/CodeQL、Rust旧main INTERNAL | EVIDENCE_NOT_RUN / EVIDENCE_FAILURE | 新扫描与匹配栈/阶段证据；不重跑无新假设的六次旧实验 |
| 源码同步 | SAFETY_NOT_CLEARED | 原create_tree准确scope/正规解除；不换API/push/sourceZIP/执行器 |

完整 A–N 与原 A–M/P0–P5 尚未完成。Browser、8region图像、Canvas、数值边界、
单Go Core、已有Write清退与受控检索成果继续保留。测试细节和失败见 QA，不在本页累积日志。

## 五个独立出口

MERGED_MAIN、VERIFIED_MAIN、COMPLETE_PRODUCT、FORMAL_RC、PUBLIC_RELEASE：均未达成。
用户目标包含满足条件后普通push原分支、准确候选CI/审查/原生验收、正常expected-head merge及正式发布。
该目标没有解除 `PAYLOAD_SCOPE_UNKNOWN / NOT_CLEARED`，也没有赋予现有资源发布资格。
旧4343私有DMG仍为 `development_dirty_non_publishable`、ad-hoc、`NOT_INSTALLED`；
后来clean提交不会更改旧seal。资源许可、Developer ID/公证、release gate分别处理。
无Goal、自动续跑、新任务或并行writer，不强推、不改历史、不直推main、不绕门禁。

## 交接最小合同

记录 Repository / Branch / START / SOURCE / DELIVERY / tree / dirty / writer / actual changes /
commands and exits / evidence scope / remote and CI checkout / failures / blockers / next action。
`matched0`、空日志、fixture、旧检查或终止exit0都不等于行为通过。
只有新增合法源码或有效证据才输出新报告包；未交出writer时不授权下一方source-write。

## Historical entry anchors

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
