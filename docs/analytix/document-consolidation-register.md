# Analytix 文档整理与归并登记

Status: Operational / reference
Applies to: 项目内 Markdown 的分类、入口、历史保留和碎片治理
Current as of: 2026-09-09
Source of truth: `docs/analytix/README.md` 的证据治理规则和当前文件树
Supersedes: 不直接废弃任何现有文件；本登记为后续逐项归并入口

## 1. 为什么不直接大规模移动

2026-08-26 的 Git 跟踪树快照共有 335 个 Markdown/MDX，其中 `docs/analytix`
95 个、OpenSpec 71 个；这些是历史数量，不是当前分母。
插件、skills、包内文档另有大量契约和参考材料。若一次性移动：

- 现有 repo-relative 链接、脚本检查和外部引用会断裂；
- Git blame 与历史审计成本上升；
- historical evidence 可能被误写成当前规范；
- 脏工作区会进一步扩大，掩盖产品代码变更。

因此本轮采取“先建立权威入口和分类登记，再按独立 change 逐项迁移”的策略。

## 2. 目录职责

| 路径 | 分类 | 规则 |
| --- | --- | --- |
| `AGENTS.md` | Operational | 仓库施工规则；每轮必读。 |
| `docs/analytix/README.md` | Operational index | 文档与证据权威总入口。 |
| `docs/analytix/specs/01-11` | 按 registry 分类 | 以 `specs/README.md` 为准；05 是 Historical reference，其余按各自接受范围使用，不能统称现行规范。 |
| `openspec/specs/` | Normative scoped | 已接受 scoped requirements。 |
| `openspec/changes/*` | Target / work plan | active change 是目标和任务账，不是实现证明。 |
| `docs/analytix/handovers/` | Operational snapshot | 跨线程接续；后续交接 supersede 前一份 currentness。 |
| `docs/analytix/qa/` | Runbook + dated evidence | README 为入口；日期报告只对记录环境有效。 |
| `docs/analytix/benchmarks/` | Reference + dated evidence | benchmark 必须绑定 commit、输入、命令、结果和 skips。 |
| `docs/analytix/upstreams/` | Reference ledger | intake、审计、吸收和 benchmark 分开表达。 |
| `docs/analytix/runtime/` | Operational/runtime reference | 当前命令和合同需重新核验。 |
| `docs/legacy/` | Historical | 只作为迁移/来源历史，不作为当前实现。 |
| `重构升级方案.md` | Historical decision ledger | 只读顶部 currentness；长编号章节不能覆盖代码/spec。 |
| `plugins/*/references/` | Plugin reference | 不等于 production contract；重复副本需哈希核对。 |
| `build/`、`dist*`、`output/`、`release/legacy/` | Generated/historical | 不作为 maintained source 或当前验证入口。 |

## 3. 高风险大文档

这些文档体量大、历史层次多，最容易被全文搜索误判。现阶段保留原路径，不做
内容重写：

| 文件 | 约行数 | 风险 | 当前入口 |
| --- | ---: | --- | --- |
| `重构升级方案.md` | 14,742 | 多时期互相矛盾的阶段结论 | 只读顶部状态，并回到代码/spec |
| `upstreams/reasonix-sync.md` | 12,888 | 长期同步流水和历史 pin | `upstreams/README.md` + 最新 marker |
| `upstreams/conflict-decisions.md` | 12,214 | ADR 与历史决策混杂 | 按 decision id 查阅 |
| `qa/release-evidence-gate-2026-06-21.md` | 11,278 | 文件名像最终门，但仅为日期证据 | `qa/README.md` |
| `upstreams/go-runtime-conformance.md` | 8,814 | 含 Go cutover 前历史假设 | 当前 Go 代码/tests |
| `specs/08-upstream-absorption-and-go-runtime.md` | 8,552 | 规范长、含历史解释 | 规范目标，不是 current pass |
| `benchmarks/upstream-scorecard.md` | 8,223 | 多批次主观分数和旧结论 | 未来 CapabilityBenchmarkV1 |
| `upstreams/kun-sync.md` | 7,362 | 历史同步流水 | 当前 source marker |
| `upstreams/absorption-ledger.md` | 6,975 | 旧批次完成语气 | active OpenSpec tasks |
| `specs/09-agent-quality-product-benchmark.md` | 6,715 | 目标 benchmark | 可执行 benchmark artifact |

## 4. 已发现的碎片与重复

1. 资金插件 `references/` 与 owner skill 下的 `references/` 有 25 组同名
   文件；2026-08-26 在 `c31dc3afb7a4be1d5193dd4085a0e434074dfbf0`
   跟踪树上按同名文件逐组执行 `shasum -a 256`，结果为 25 组一致、
   0 组差异。这不是
   可直接删除的无效重复：根 `references/` 是发布校验和脚本 source，
   `skills/analytix-fund-analysis/references/` 是 skill 打包副本，README 与
   focused gate 明确要求它们字节一致。在生成/同步和消费者合同未改变前，
   保留两个位置并把“字节一致”作为可执行门禁。
2. upstream、QA、benchmark 多份日期报告使用 `final`、`closure`、`passed`
   命名。保留原始证据，但引用必须经过目录 README 的 currentness 规则。
3. `docs/legacy/kun` 包含中英文历史材料。已增加 `docs/legacy/README.md`，
   不再让搜索结果看起来像当前 Analytix 规范。
4. `.agents`、`.claude`、`.cursor`、`.kunsdd` 内的 Markdown 属于工具/工作流
   配置，不应并入产品规范。
5. 包内 `node_modules` 的 README/LICENSE 是依赖材料，不纳入产品文档数量和
   整理范围。

## 5. 后续整理队列

按实际影响验证整理涉及的链接和消费者。相关文档可合并修订；本队列不是产品交付的前置门禁，也不要求每项另建 change、切片或全量验证。

| 优先级 | 动作 | 最小完成证据 |
| --- | --- | --- |
| P0-doc | 为会被当作当前入口的旧 `final/closure` 报告加 historical banner | 无正文改写；链接检查通过 |
| P1-doc | 把 `重构升级方案.md` 的 current-state 摘要拆为短索引，原文件保留 decision ledger | 所有旧 anchor 可追踪 |
| P1-doc | 为 upstream 大 ledger 生成按 source/date/decision-id 的机器可读索引 | marker 与 commit 校验 |
| P1-doc | 将插件 reference 镜像收敛为明确的单源生成/同步流程，不直接删除消费者要求的副本 | 生成器或同步命令、运行时路径测试、全量哈希 |
| P2-doc | 将 QA 证据元数据统一为 commit/platform/command/result/skips | checker 验证 |
| P2-doc | 从 scorecard 迁移到 `CapabilityBenchmarkV1` 原始结果索引 | 可执行测试绑定，禁止主观 exceeded |
| P2-doc | 清理过期双语文档的 currentness 漂移 | 中英文同步、链接检查 |

## 6. 新文档写入标准

关键文档按需要说明以下信息；已由目录明确分类的历史材料无需逐文件增加重复字段：

```text
Status:
Applies to:
Current as of:
Source of truth:
Supersedes / Superseded by:
```

每个 present-tense PASS 必须包含精确命令、commit/worktree、环境、退出结果和
skip 状态。仅规划、只读审计、fixture、历史包或外部条件缺失必须显式标记。

本机绝对路径只允许作为来源/交接 provenance；不得写凭据、真实案件数据、
完整银行账号、完整 PII、reasoning 或原始外部错误正文。


## 2026-09-09 本轮处置

本轮 [审阅记录](documentation-delivery-review-2026-09-09.md) 修正当前入口的
Hub-first 冲突、插件身份归属与规范分类。文档地图中的 2026-07-29 外部指南比较
迁至 [Historical reference](upstreams/agent-guidance-review-2026-07-29.md)，
原入口保留链接。历史报告、归档 change 和打包要求的 reference 镜像保留；
不会因为文件长、旧或重复就删除仍被消费或用于证据追溯的内容。
