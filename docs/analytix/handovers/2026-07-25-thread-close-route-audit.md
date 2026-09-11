# 2026-07-25 线程收工与 Agent 产品路线复核

Status: Historical operational handover
Applies to: 源线程 `019f4ac7-e9cf-7522-be7a-6ddba8d26d02` 的收工检查点
Current as of: parent HEAD `0a3f1374b1e08cbb1135f8aadaddb91e505d1226`
加本次检查点提交前工作区
Source of truth: 当前代码、active OpenSpec tasks、本文记录的实际命令结果
Supersedes: 本文补充
[`2026-07-25-first-stage-pause.md`](2026-07-25-first-stage-pause.md)，但不把历史
验证升级为当前 PASS

## 1. 结论

Analytix 的技术路线没有根本偏离现代 Agent 产品：

- 桌面产品仍是 `Electron + React + TypeScript`；
- 唯一生产 Agent Core 仍是 `packages/runtime-go`；
- thread、turn、tool、approval、subagent、Todo、persistence、HTTP/SSE 是正确
  平台底座；
- 同案、同轮、同 epoch、同 snapshot 的宿主 Evidence Receipt、Final Evidence
  Gate、PII controlled artifact 和 Publication Receipt 是合理的案件分析差异化；
- 没有恢复 TypeScript Agent Runtime，也没有引入第二套 Rust Agent Runtime。

但当前施工顺序存在结构性失衡：安全、恢复、native authority 和发布证明的
深度，已经明显超过普通 Agent happy path、资金插件正向事实链和正式打包实例的
可用性证明。继续增加安全对象而不先打通真实用户链路，会偏离“最强 Agent
工作平台”目标。

本次提交是未完成 Goal 的可恢复检查点，不是第一阶段完成、P0 完成、资金插件
交付完成或正式发布授权。

## 2. 当前任务与验证事实

active change `case-evidence-publication-gate` 的当前账面为：

| 阶段 | 完成 / 总数 | 当前结论 |
| --- | ---: | --- |
| P0 | 52 / 54 | `2.14`、`2.18` 未关闭 |
| P1 | 42 / 65 | 证据内核已落地很多，但 production composition 与全矩阵未闭合 |
| P2 | 0 / 23 | 正向 funds/data/PII/report 任务未关闭 |
| P3 | 4 / 23 | 主要是 intake/白盒审计，不是能力吸收完成 |
| P4 | 0 / 16 | 当前 package、GUI、Provider、release 指标未关闭 |
| 全部 | 105 / 198 | 唯一 Goal 未完成 |

最新可用验证与未验证边界继续以
[`2026-07-25-first-stage-pause.md`](2026-07-25-first-stage-pause.md) 为准：

- `internal/runtimeapp` 和 `internal/server` focused Go package 曾在该工作区切片
  上 PASS；
-完整 ordinary Go 结果丢失，不能计为 PASS；
- production tag、race、正式 package 和最新 GUI 未在最终工作区完整重跑；
- 资金 0.16.16 的多项 containment/schema/oracle contract 有 PASS；
- credentialed real-case、installed frontdoor、正式 package 和正向事实 E2E
  仍是 `UNVERIFIED` 或 `NOT IMPLEMENTED`。

本次收工提交前新增的 fresh 结果：

| 命令/范围 | 结果 |
| --- | --- |
| `npx openspec validate case-evidence-publication-gate --strict` | PASS |
| `npm run typecheck` | PASS |
| cache/GOMODCACHE focused Vitest | PASS，2 files / 9 tests |
| `npm run funds:p0:contracts` | PASS |
| `npm --prefix packages/runtime run test` 初次默认并发 | FAIL，100 files PASS、4 files FAIL；10/1100 tests 因 5 秒超时和超时后的 temp cleanup 失败 |
| 上述 4 个失败文件，single worker | PASS，4 files / 156 tests |
| 完整 Runtime suite，single worker | PASS，104 files / 1100 tests |
| repository link + Obsidian wiki-link check | PASS |
| `git diff --check` | PASS |

默认并发失败不是功能 PASS，也不能被 single-worker PASS 从历史中抹掉。由于项目
强制把 test temp/cache 放到外置 `/Volumes/AnalytixCache`，本次把
`packages/runtime` 的 canonical test script 固定为 single worker、关闭 file
parallelism；完整等价命令已 fresh 通过。这是确定性测试调度修复，不放宽断言或
test timeout。

## 3. 这条线程实际推进了什么

### 3.1 已形成的有效底座

- Go Runtime 对 reasoning、历史投影、事件持久化、accepted final、execution
  grant、工具广告/执行一致性、dataset snapshot、attachment、subagent 和恢复
  路径增加了大量 fail-closed 约束与故障测试；
- Electron 启动代码增加单实例锁失败即退出的分支，packaged app 不再使用
  development-server hint；
- Go Runtime 预热/启动不再因为尚未配置模型 API key 而被桌面层提前阻断；
- 本地开发 native 与正式发布 authority 被分开，本地产物明确保持
  non-publishable，release/publish 路径继续要求正式授权；
- 资金插件源码版本为 `0.16.16`，默认 MCP 配置关闭，production 事实工具在
  证据权威尚未组合时保持零广告；
- 可再生 npm/Go/Rust/Python/Playwright 临时缓存已建立
  `/Volumes/AnalytixCache/development` 路由和 fail-closed helper；
- 项目文档地图、历史文档分类、暂停交接、任务/验证台账和 Obsidian 导航已建立。

### 3.2 仍不能冒充完成的部分

- `packages/runtime-go/internal/runtimeapp/app.go` 中
  `datasetSnapshotAuthorityV2` 仍未组合；在它为 `nil` 时保持 funds facts
  禁用是正确止血，但不构成正向资金分析能力；
- `.mcp.json` 的 production funds server 仍为 disabled，工具列表为空；
- 还没有证明“用户请求 -> Agent 选中 funds -> 当前运行 source probe ->
  immutable snapshot -> EvidenceReceipt -> ClaimRecord -> Final Gate -> UI /
  controlled artifact”；
- 还没有用最新源码构建并实际验证一个且仅一个 packaged GUI、一个 Go backend、
  health、Agent 请求、正常退出和无残留进程；
- 30 分钟 runtime startup timeout 为大型语义迁移留出空间，但如果没有阶段
  进度、取消、明确 blocker 和 watchdog，它会被用户感知为无限 loading；
- cache helper 目前依赖操作者先 `source`，root `npm run dev/test/build` 没有
  机械强制继承该环境；忘记 source 仍可能回到系统盘；
- Runtime Vitest 在外置 cache/temp 上的默认 file parallelism 会触发 5 秒超时
  和 cleanup 竞态；canonical Runtime test 已改为 single worker，但 root suite
  和其他工具仍需分别验证其并发/缓存行为；
- report P2 `7.1-7.6` 未关闭，所以不能只挑 P4 `9.5` 就宣称 report E2E 完成；
- 多个 receipt、witness、CAS、reconcile 和 authority 类型必须由一张 production
  composition map 和一条 E2E call graph 证明只有一个权威主链，否则存在平行
  安全机制与层间 fail-open 风险。

## 4. 与 Codex、Claude Code、OpenCode 的红线复核

本节比较的是可观察的产品机制，不是品牌或模型主观评分。

### 4.1 现代 Agent 平台的最低竞争基线

Codex 当前公开产品面包括：

- App Server 的 thread start/resume/fork/read、turn、streamed items、approval、
  compaction、Goal、skills reload/change notification 和 MCP
  tool/resource/auth status；
- desktop worktree 与 Local/Worktree handoff；
- 默认启用的并行 subagents；
- lifecycle hooks 对 shell、file edit、MCP 和其他本地工具的拦截。

来源：

- [Codex App Server](https://learn.chatgpt.com/docs/app-server)
- [Codex Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
- [Codex Worktrees](https://learn.chatgpt.com/docs/environments/git-worktrees)
- [Codex Hooks](https://learn.chatgpt.com/docs/hooks)

Claude Code 当前公开产品面包括独立上下文/工具/权限的 subagents、worktree
isolation、skills、MCP、agent teams，以及覆盖 session、tool、permission、
subagent、task、compaction、worktree 和 failure 的 lifecycle hooks。

来源：

- [Claude Code Subagents](https://code.claude.com/docs/en/sub-agents)
- [Claude Code Hooks](https://code.claude.com/docs/en/hooks)
- [Claude Code Features](https://code.claude.com/docs/en/features-overview)

OpenCode 当前公开产品面包括 built-in/custom/MCP tools、allow/ask/deny
permissions、per-agent tool policy、subagents、Todo、on-demand skills、LSP、
terminal/desktop/IDE surfaces 和 plugin event extension。

来源：

- [OpenCode Tools](https://opencode.ai/docs/tools)
- [OpenCode Permissions](https://opencode.ai/docs/permissions)
- [OpenCode Agents](https://opencode.ai/docs/agents/)
- [OpenCode Skills](https://opencode.ai/docs/skills/)

### 4.2 Analytix 当前相对位置

| 维度 | 当前 Analytix | 路线判断 |
| --- | --- | --- |
| thread/turn/SSE/approval/Todo/subagent | 已有较广合同和大量测试 | 方向正确；需真实 packaged happy path、恢复和 UX 证明 |
| tool/MCP 权限 | execution grant、广告/执行一致性、source probe 约束较深 | 案件安全可能形成优势；需 current-run E2E、MCP auth/status 和失败 UX |
| evidence/publication/PII | 目标和实现深度高于通用 coding agent 的案件取证范围 | 差异化正确；production composition 与正向 facts 尚未闭合 |
| skills/hooks | 有 skill surface；TypeScript hooks 主要位于 contract/test support | 与 Codex/Claude/OpenCode 的生产 lifecycle extensibility 仍有差距 |
| worktree/handoff | 有受约束 subagent worktree 基础 | 缺少被实际验证的用户级 Local/Worktree handoff 与恢复体验 |
| packaged desktop | 有大量 authority/packaging contract | 用户已观察到重复前端和 backend 未启动；最新真实包未验收 |
| funds | 安全 quarantine 较强 | 正向业务能力未交付，不能算领先 |
| provider/cache | stable prefix 和 provider 合同已有基础 | DeepSeek cache superiority 没有同定义 benchmark |
| release completeness | fail-closed 设计深入 | macOS/Windows 当前正式实例和更新链未形成 fresh 证据 |

结论：Analytix 在“高风险案件证据发布控制”的设计深度上有潜在领先点，但目前
仍落后于成熟 Agent 产品已被用户验证的日常工作流完整性。红线不是删除证据层，
而是先把基础 Agent 平台做到稳定、可观察、可恢复，再用确定性 benchmark 证明
差异化。

## 5. 上游审计的 currentness 风险

本机只读复核显示：

| checkout | 本地分支状态 |
| --- | --- |
| `CodexDesktop-Rebuild` | 与已知 `origin/master` 对齐 |
| `claude-code` | `origin/main` 落后 1 个提交 |
| `claw-code` | 与已知 `origin/main` 对齐 |
| `gajae-code` | `origin/main` 落后 46 个提交 |
| `hermes-agent` | `origin/main` 落后 35 个提交 |
| `lazycodex` | 与已知 `origin/main` 对齐 |
| `opencode` | 与已知 `origin/dev` 对齐 |
| `DeepSeek-Reasonix` | `origin/main-v2` 落后 4 个提交 |

这里的“对齐”只表示本地 remote-tracking ref，没有在本次收工提交中执行
`git fetch`，不能证明 GitHub 此刻没有新提交。现有 upstream audit 必须标记其
source commit；下一阶段如果进入 P3，应先在授权范围内 fetch/pull，再重新做
代码、文档、测试和许可证矩阵。本次第一阶段不得顺带展开全量上游吸收。

## 6. Obsidian 与项目文档复核

Obsidian vault 当前是知识/交接层，不是 Git 仓库，也不是代码或任务权威。当前
结构已经覆盖：

- 导航；
- 第一阶段暂停交接；
- Goal/P0-P4 与验证台账；
- 证据优先架构；
- funds 0.16.16 状态；
- 上游吸收现状；
- 产品路线审计；
- 下一线程说明；
- 文档治理。

现有 wiki links 已进行解析检查，未发现失效目标；文档没有保存真实案件数据、
凭据、完整银行账号或 reasoning。后续仍需遵守：

- repository code/spec/tasks/fresh validation 是 source of truth；
- Obsidian 只保存摘要、索引、风险和恢复步骤；
- 每次阶段提交后更新 checkpoint hash 与验证新鲜度；
- 不把 `final`、`closure`、旧 PASS、文件存在或 mock 结果写成当前产品完成；
- 不复制大量源码/日志/测试输出到 vault。

## 7. 下一阶段正确施工顺序

下一线程必须按依赖闭合，而不是同时铺开 P0-P4：

1. **恢复证据**：source cache helper，复跑最后 cache contract，取得完整 Go
   ordinary/prod/race 和 root/runtime/funds 命令的可读取结果；
2. **关闭 P0 `2.14`、`2.18`**：installed frontdoor、formal package、
   credentialed real-case 分开记录，外部条件不足就保持 `UNVERIFIED`；
3. **恢复 packaged 核心可用链**：一个实例、一个 renderer、一个 Go backend、
   health、最小 Agent/tool/thread/Todo/subagent/compaction、shutdown；
4. **完成正向 funds 的最小依赖闭环**：先组合 DatasetSnapshotV2 和 host
   evidence authority，再启用任何事实工具；完成 lineage、claim、PII、
   Final Gate 和 controlled artifact；
5. **只做上述链路必需的 P1/P2/P4**：特别是 `3.6/3.7/3.7a`、
   `5.6/5.7/5.11/5.12`、资金 `6.*` 的实际依赖、报告 `7.1-7.6` 的实际依赖、
   `9.5/9.6/9.8-9.11/9.11c`；
6. **立即收工**：第一阶段通过后停止，不自动进入 P3、全平台发布或“超过上游”
   宣称。

更完整的可复制提示词保存在 Obsidian：
`Analytix/07-后续施工/下一线程完整施工提示词.md`。

## 8. 线程标识警告

用户声明的源线程 ID 是：

```text
019f4ac7-e9cf-7522-be7a-6ddba8d26d02
```

用户同时提供的 deep link 是：

```text
codex://threads/019f8e86-4d84-7a70-ae58-001ef0674459
```

两者不是同一个 UUID。本次不推断它们等价，也不改写任一值。新线程只能把前者
作为本交接的源线程标识，把后者作为用户提供的人工定位链接；需要恢复旧线程
时应先由用户核对目标。
