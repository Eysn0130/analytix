# 2026-07-25 第一阶段施工暂停交接

Status: Historical operational handover
Applies to: 2026-07-25 暂停时的本机 `main` 工作区
Current as of: HEAD `0a3f1374b1e08cbb1135f8aadaddb91e505d1226` 加未提交修改
Source of truth: 当前 Git 工作区、active OpenSpec tasks、本文记录的实际命令结果
Superseded by: 后续日期更晚且重新核验当前工作区的交接

## 1. 暂停决定

用户已明确要求暂停一切代码施工、测试、打包和发布推进。本线程在收到该要求后：

- 不再修改产品代码；
- 不再勾选 OpenSpec 任务；
- 不再启动新的测试、构建、打包或发布；
- 不创建新 Goal，不把现有唯一 Goal 标记为 complete 或 blocked；
- 仅进行只读审计、Obsidian 同步和项目文档治理；
- 检查后未发现本线程遗留的 `go test`、Vitest 或 packaged-session-soak
  施工进程。

本文件记录暂停点，不表示第一阶段闭环，也不表示 P0-P4 完成。

## 2. 唯一 Goal

唯一 Goal 的产品目标是：把 Analytix 升级为证据优先、宿主强制、跨案件隔离、
可审计、可验证、可安全发布的高风险案件分析平台；完成 Go Runtime 和资金分析
插件 P0-P4；吸收指定上游能力；以确定性能力矩阵和完整桌面、打包、发布证据
证明结果。

Goal 工具在暂停时的状态为 `usageLimited`，累计 token 使用记录为
`164932156`。这只是执行系统状态，不是产品完成状态。Definition of Done 未
满足，因此不得调用 Goal complete。

## 3. Git 与工作区基线

暂停时观察值：

```text
branch: main
HEAD: 0a3f1374b1e08cbb1135f8aadaddb91e505d1226
tracked modified files: 293
untracked status entries: 87
git diff --stat: 293 files changed, 18114 insertions(+), 3793 deletions(-)
```

主要变更分布：

| 区域 | 已跟踪修改文件 | 已跟踪 diff 规模 | 说明 |
| --- | ---: | ---: | --- |
| `packages/` | 204 | `+9272 / -1296` | Go Runtime 为主，也包含 TS 契约和测试。 |
| `src/` | 26 | `+2244 / -210` | Electron main、runtime adapter、renderer 契约。 |
| `scripts/` | 19 | `+3370 / -705` | native authority、打包、缓存、验证脚本。 |
| `plugins/` | 16 | `+1462 / -1088` | 资金插件 0.16.16 的 MCP/contract/oracle。 |
| `backend/` | 9 | `+1062 / -246` | 数据与外部 runtime 边界。 |
| `tools/` | 15 | `+672 / -247` | 既有 Rust 数据平面工具，不是第二 Agent Runtime。 |

此外有 87 个未跟踪状态条目，包括新的 Go domain/app/adapter/tests、脚本和
`dist-local-nonpublishable/`。后者是旧的本地非发布包，不是当前源码或当前包
验证证据。整个工作区属于用户现有修改；恢复施工时禁止 reset、checkout、
clean、全量格式化或用旧包覆盖。

## 4. OpenSpec 与任务基线

`openspec list --json` 在暂停时返回：

| Change | 状态 | 完成 / 总数 |
| --- | --- | ---: |
| `case-evidence-publication-gate` | `in-progress` | `105 / 198` |
| `refresh-upstream-audit-governance` | `complete` | `17 / 17` |

统一 change 的任务统计：

| 阶段 | 总数 | 完成 | 未完成 |
| --- | ---: | ---: | ---: |
| P0 | 54 | 52 | 2 |
| P1 | 65 | 42 | 23 |
| P2 | 23 | 0 | 23 |
| P3 | 23 | 4 | 19 |
| P4 | 16 | 0 | 16 |
| P0-P4 合计 | 181 | 98 | 83 |
| 阶段外 0/10 | 17 | 7 | 10 |
| 全部 | 198 | 105 | 93 |

此前出现的 `97 / 193`、`90 / 176` 是旧版 `tasks.md` 的时点数字；之后同一个
change 增加了有证据缺口的子任务。当前唯一有效分母是实际文件和 CLI 返回的
`105 / 198`。不能引用旧分母，也不能为提高进度感静默改变任务归属。

P0 仍未关闭：

- `2.14`：需要在最终当前工作区上重新审核全部 P0 diff、fresh test 和正式
  packaged-instance 证据；
- `2.18`：需要重跑全部资金 contract/smoke/oracle/red-team，并把 credentialed
  real-case、installed frontdoor 和正式 package 单独验证。

因此“全部 P0 已完成”“第一阶段闭环”均不是当前事实。

## 5. 暂停前最后一批实现

以下是暂停前已经进入工作区、但尚未形成整体交付闭环的主要切片。

### 5.1 Go Runtime

- private CAS prepared recovery 增加语义恢复屏障，相关文件集中在
  `internal/adapters/outbound/finalauthority` 和 `internal/runtimeapp`；
- `internal/server/usage_test.go` 从固定 100ms 延迟代理改为确定性锁钩子；
- 继续收紧事件、reasoning、SSE、持久化、execution grant、attachment、
  dataset snapshot、publication、subagent 和历史迁移边界；
- `internal/server` 仍是较大的过渡 facade。恢复施工时，新独立行为仍应落在
  domain/app/ports/adapters，不应继续扩大 facade。

### 5.2 开发缓存与 native build

- `scripts/use-analytix-cache.sh` 和
  `scripts/lib/development-cache-environment.cjs` 把可再生开发缓存路由到
  `/Volumes/AnalytixCache/development`；
- source 侧正在修复 after-pack 的 `GOMODCACHE` 权威：只允许 marker 授权的
  精确缓存根穿过 hermetic boundary；ambient `GOMODCACHE` 继续拒绝；
- host resolver 已成功解析到
  `/Volumes/AnalytixCache/development/go-mod`；
- 最后一个 focused test 只因旧错误文案断言不匹配而失败，断言已修改，但暂停
  后没有复跑。因此该切片状态是 `UNVERIFIED AFTER FINAL PATCH`，不是 PASS。

### 5.3 资金插件

- 当前源码版本为 `0.16.16`；
- production MCP 默认关闭事实工具、工具列表为空，保持 P0 fail-closed；
- DuckDB、安全 schema、MCP error、snapshot manifest、production entry
  closure、report receipt、cancellation 等 contract 已获得多项 focused PASS；
- 正向事实链仍未接入生产组合，详见第 7 节。

## 6. 当前有效验证证据

以下仅对记录的当前脏工作区切片有效。

### 6.1 Go focused package

| 命令/范围 | 结果 |
| --- | --- |
| `go test -p=1 -count=1 -timeout=30m ./internal/runtimeapp` | PASS，794.783s |
| `go test -p=1 -count=1 -timeout=30m ./internal/server` | PASS，1186.333s |
| ordinary 全矩阵 `go test -p=1 -count=1 -timeout=40m ./...` | `RESULT LOST`；运行过，但会话消失，无法取得最终退出状态 |
| production tag 全矩阵 | 未在最终当前代码上执行 |
| race 全矩阵 | 未在最终当前代码上执行 |

`tasks.md` 中更早的完整 Go ordinary/prod/race PASS 是历史时点证据。当前有后续
修改，不能自动继承为暂停时工作区的完整 PASS。

### 6.2 资金插件 focused 证据

| 验证 | 结果 | 边界 |
| --- | --- | --- |
| `funds:p0:contracts` | PASS，5/5 umbrella；schema 62/62；identity 10/10 | 结构与 P0 contract |
| doctor | `ok:true`，有 skill surface 和旧安装投影警告 | 不证明正式包 |
| health | PASS，12/12 | 明确生产工具为零、source unavailable |
| MCP lifecycle | PASS | containment |
| MCP surface mutation | PASS，7/7 | fail-closed |
| CashClassificationVectorsV1 | PASS，12/12 | 真实 DuckDB oracle |
| P0 containment | PASS | 边界 |
| production MCP entry closure | PASS | 8 modules、hostile loader 24、contract paths 5 |
| DuckDB external result safety | PASS | focused |
| ReportRequiresPublicationReceipt | PASS，30 checks | receipt boundary |
| nested cancellation | PASS，2 checks | focused |
| frontdoor containment | PASS，5/5 + 7/7 | 不是正向事实链 |
| MCP return oracle | PASS，5 checks | security boundary |
| real-case/no-case oracle | 进程退出 0，但业务 JSON 为 `status=failed` | `UNVERIFIED/BLOCKED`，不得写成 PASS |
| functional eval `--skip-backend` | 35/36 | 当时缺少外部 skill baseline；需按现已安装版本重新核验 |

### 6.3 无效或中止的证据

- 一个过大的 packaging test 因同时启动多个 Go root tests 被人工终止，退出
  130；其初步 6 个失败是重叠执行产生的无效结果，状态为 `ABORTED /
  INVALIDATED`；
- 最后一处缓存断言修改后没有复跑；
- 没有在最新工作区重新构建、启动和验证 packaged app；
- 没有完成 macOS 正式签名/公证或 Windows approved host 验证。

## 7. 资金分析插件 0.16.16 的真实状态

### 已成立

- P0 失败链是机械性的：无 host registry、无 source readiness、无 evidence
  receipt 时不广告事实工具，也不生成案件事实；
- production `tools/list` 为空，插件 `.mcp.json` 关闭，Go MCP manager 保持
  quarantine；
- missing/null/unknown 不应被升级为零；
- schema、MCP error、DuckDB containment、报告 receipt 和多项恶意输入测试
  已有 focused 证据。

### 尚未成立

- Go host-native funds producer / ingest / witnessed evidence registry 的正向链；
- DatasetSnapshotV2 authority 在 production composition 中的事实签发；
- SharedEvidence / ThreadRisk 凭据 loader 到生产组合的完整 wiring；
- 从用户资金分析请求，到 Agent 选择工具、执行真实数据、签发同案同轮同快照
  receipt、生成 claim、通过 Final Gate、UI/受控 artifact 展示的正向 E2E；
- credentialed real-case；
- 正式 packaged-instance 中 0.16.16 的完整 materialization 与正向验证；
- 完整 PII 仅进入受控 artifact 的生产授权闭环。

结论：当前资金插件是“P0 安全隔离有效、正向事实能力未交付”。零工具 quarantine
是安全状态，不是资金分析产品交付完成。

## 8. 桌面启动与打包

- `dist-local-nonpublishable/mac-arm64/analytix.app` 是早期本地开发包，authority
  为 `development_dirty_non_publishable`、ad-hoc signed、无 Team ID；
- 该包早于最后一批源码修改，不能作为当前正式包或当前 GUI 健康证据；
- 用户报告过重复拉起两个前端，其中一个样式错误，另一个只有前端且 Go Agent
  未启动。暂停前尚未在最新代码上完成单实例、backend 自动启动、health 和
  全进程退出的修复验收；
- release/package/publish 仍应保持 fail-closed；
- 原始默认数据未直接做试验性迁移。已知备份/诊断副本为：
  `/Users/sun/AnalytixStage1Backup-20260723` 和
  `/Users/sun/AnalytixStage1DiagnosticV7-20260723`；
- 没有在最新包上完成默认数据副本的线程、Todo、记忆、工具记录、资金结果和
  回滚兼容验证。

## 9. 上游吸收状态

OpenSpec 8.1、8.3、8.4、8.5 已勾选，表示完成来源 intake 或白盒审计，不表示
Analytix 已实现或超过上游。

| 来源 | 已完成 | 未完成 |
| --- | --- | --- |
| `_upstreams` 11 个登记项目 | 2026-07-20 source/license/currentness intake | 全量递归能力审计 8.2、实现 8.7-8.13、矩阵和 benchmark |
| Data Analytics `0.2.8-13ceeea1f599` | pinned audit；focused 89/89 | Analytix 落地 8.14、BenchmarkV1 |
| Investment Banking `0.1.29` | pinned audit | 上游自身 focused 47/50；Analytix 落地 8.15 |
| Public Equity Investing `0.1.31` | pinned audit | 上游自身 focused 48/78；Analytix 落地 8.16 |
| postgres-mcp | source/white-box audit | 本机缺 `pglast` 环境，未运行上游测试；Analytix 8.17 未落地 |
| Instructor | source/white-box audit | 本机依赖不完整，未运行全部上游测试；Analytix 8.18 未落地 |

`UpstreamCapabilityMatrix`、`CapabilityBenchmarkV1`、DeepSeek cache superiority、
Codex/Claude Code agent-platform matrix 和 CodexDesktop-Rebuild completeness
matrix 均未完成。当前没有确定性证据支持“Analytix 已超过 Codex、Claude Code、
OpenCode 或全部上游”。

## 10. 产品方向审计

### 没有偏离的部分

- 坚持 `Electron + React + TypeScript` 桌面壳和唯一 Go Agent Core；
- HTTP/SSE、thread、tool、approval、subagent、Todo、persistence 等是现代 Agent
  平台的正确基础；
- 证据 receipt、Final Gate、跨案件隔离、reasoning 隐私和发布 authority 是
  Codex/Claude Code/OpenCode 不以案件取证为中心时，Analytix 应形成的差异化；
- 没有恢复 TypeScript Agent Runtime，也没有引入第二 Rust Agent Runtime。

### 当前偏航风险

1. **安全基础设施压过可用主链。** 大量精力进入 native release authority、
   private CAS、recovery、历史迁移和 cryptographic witness，而普通用户的
   Agent happy path、插件正向调用和 packaged backend 启动仍未闭环。
2. **“零工具安全”容易被误报为“插件完成”。** 它防止幻觉，但用户仍无法完成
   真实资金分析。
3. **脏工作区过大。** 293 个已跟踪修改文件和 87 个未跟踪条目让归因、回归、
   review 和 rollback 变得困难。
4. **权威层数量过多。** 多个 receipt/index/witness/CAS/reconcile 切片如果没有
   一条清晰 composition path，会形成平行安全机制。
5. **阶段范围依赖不闭合。** 第一阶段选择的 P4 `9.5` 依赖仍未完成的 P2
   报告发布 `7.1-7.6`，说明仅按编号挑任务无法形成最小可交付切片。
6. **文档完成语气过多。** 多份带 `final`、`closure`、`passed` 的历史快照会让
   新线程误判当前状态。
7. **产品比较尚停留在目标。** 没有在真实 coding-agent 工作流上比较工具成功率、
   线程恢复、延迟、MCP、subagent、权限和桌面完整度。

## 11. 恢复施工时的最小顺序

用户当前要求暂停；以下只是下一线程建议，不授权自动施工。

1. 再次冻结并盘点工作区，确认没有新修改或遗留进程。
2. 只复跑最后一处 cache/GOMODCACHE focused tests，修复或回退该局部漂移。
3. 重新运行 final-worktree Go ordinary/prod/race 矩阵；结果不可复用历史报告。
4. 完成 P0 `2.18` 的当前 0.16.16 contract、installed frontdoor 和正式 package
   证据；真实 case 无条件时标 `UNVERIFIED`。
5. 完成 P0 `2.14` 总审并明确 P0 关闭或仍开放。
6. 在两条后续路线中只选一条：
   - **A：可用安全边界。** 一个 packaged app、Go backend 自动启动、普通 Agent
     非案件工具链、线程/Todo/subagent/记忆/compaction，资金事实继续 boundary-only；
   - **B：资金正向事实链。** 先实现 host-native ingest/snapshot/evidence/claim/
     final/publication authority，再做 UI 和 package E2E。
7. 核心可用闭环稳定后，再启动 P3 全量上游 benchmark 和产品扩张。

不要在同一施工切片同时承诺 A、B、全上游扩张和正式跨平台发布。

## 12. 恢复前必读

- `AGENTS.md`
- `docs/analytix/README.md`
- `docs/analytix/handovers/README.md`
- 本文件
- `openspec/changes/case-evidence-publication-gate/tasks.md`
- `docs/analytix/document-consolidation-register.md`

Obsidian 中的知识库是便于检索的同步副本，不是仓库规范或代码事实来源。
