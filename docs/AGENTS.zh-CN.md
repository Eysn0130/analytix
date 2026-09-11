# analytix 文档 Agent 指南

本文件约束 `docs/` 下的文档工作。把已有 Markdown 当作当前架构或验收证据前，
先阅读 `docs/analytix/README.md`。

## 分清工作规范、目标、实现与证据

- 根目录及嵌套 `AGENTS.md` 约束 Agent 怎样工作。
- `docs/analytix/specs/README.md` 把编号 specs 分类为已接受的产品目标、验收
  合同或历史参考；`openspec/specs/<capability>/spec.md` 也保存归档同步后
  已接受的 scoped requirements。已登记 spec 中的历史审计段落不自动代表当前
  状态。
- active `openspec/changes/<change>/` 只定义该 change 的 scoped target，不是
  “已经实现”的证据。archived change artifact 是决策历史；同步进入
  `openspec/specs/` 的 requirement 仍是已接受目标。
- 当前代码、测试、package scripts 及可复现的 runtime/build 结果定义
  as-built worktree。
- 带日期的 QA、benchmark、upstream、validation、release 文档是证据快照，
  只对记录的 commit、平台、环境和命令有效。
- `docs/legacy/`、`release/legacy/`、`output/`、`dist*/`、打包 resources 和
  `vendor/` 属于历史、生成或 vendored 输入，不能从中复制当前时态的架构结论。

目标与当前实现不一致时，分别写清 `as-built`、`target` 和 `gap`，不要靠改
时态让其中一方消失。

## 当前架构锚点

具体行为仍要查当前代码；如果没有新的明确 target spec，文档不能违反以下
边界：

- renderer 使用 `window.analytix`。
- runtime-owned settings 使用顶层 `runtime`；模型 provider profiles 使用
  顶层 `provider`。
- 生产 agent core 是 `packages/runtime-go`，由
  `packages/runtime-go/internal/runtimeapp` 组合。
- `packages/runtime` 提供公共 TypeScript contracts/config/telemetry 与
  `analytix serve` Go launcher；其中退役的 loop/server/model 源码只用于测试、
  conformance 或迁移对照，不是生产扩展入口。
- `ANALYTIX_RUNTIME_BACKEND=typescript` 不能启动 TypeScript agent runtime。
- 普通 packaged 首次启动由 Analytix Hub 账号就绪状态和 main-process gateway
  credential 门控，不再由“缺少手工 API key”门控；自定义 Provider 仍是
  Settings 中的可选能力。
- canonical identity 是 `analytix`；official Standard Windows 只在 display
  copy 使用 `Analytix灵鉴`，artifact 使用 `analytix-standard-*`，不改
  executable/app id/CLI/env/protocol。
- 新交互会话的 policy 默认值是 execution-policy version 2：
  `approvalPolicy: on-request` 加 `sandboxMode: workspace-write`。
  未版本化且恰好是旧默认 `auto` 加 `danger-full-access` 的组合只迁移一次；其他
  有效选择和 version-2 full-access opt-in 保持有效。workspace-write 是应用层
  tool/path policy，不是 OS 隔离边界，并会阻止前台/后台主机 shell。无人值守
  Connect Phone 与 scheduled task 使用 `never` 加 `workspace-write`。

不能把以下已不存在的 TypeScript 路径写成当前生产入口：
`packages/runtime/src/server/runtime-factory.ts`、
`packages/runtime/src/server/routes/`、
`packages/runtime/src/loop/agent-loop.ts`、
`packages/runtime/src/adapters/model/deepseek-compat-model-client.ts`。

## 文档状态

- `Normative`：必须满足的目标或不变量。
- `Operational`：当前 runbook 或 Agent 工作方式。
- `Reference`：设计输入、对照或解释材料。
- `Historical`：保留的时点快照，不能驱动当前施工。

新的高影响文档应在有意义时说明 status、scope、source of truth、currentness
和 supersession。不要给所有旧文件机械批量加 frontmatter；优先修复真的会误导
施工的入口文档。

## 修改规则

- 发布当前时态结论前，验证路径、命令、分支、配置字段和架构 owner 仍存在。
- 优先使用 repo-relative links；本机绝对路径只放在明确的 provenance 或
  historical records 中。
- 当前行为、目标行为和历史行为分段描述。
- “schema 接受/写入某个 key”不等于“生产 runtime 使用该行为”。必须把字段
  追踪到当前 Go composition root 与 use case 后再下结论。
- 保留旧报告时，优先添加 currentness/historical banner，不改写原始结果；若
  内容泄密或带来现实安全风险，必须删除或脱敏。
- 中英文文档描述同一当前产品事实时同步更新。
- 保持改动聚焦，不顺手刷新生成证据、打包快照或无关 ledger。

## 证据规则

- 文件名含 `final`、`generatedAt` 较新或旧报告写着 `passed`，都不等于当前
  release authorization。
- 记录被测 commit/worktree、精确命令、平台、环境限制和诚实状态：`pass`、
  `partial`、`skipped`、`blocked` 或 `not_configured`。
- fixture、conformance、shadow、deterministic、fake-provider 结果不能写成
  live provider、live MCP、packaged GUI 或 operator evidence。
- 把快照改成当前时态结论前，重新运行当前相关命令。
- 不保存真实密码、private key、product key、API key、gateway token、
  remote-control ID、recovery code 或含密命令输出；使用脱敏 placeholder 与
  合规 secret storage。

## 验证

纯文档改动至少运行：

```bash
git diff --check
```

同时检查新增相对链接和引用路径。只有当文档结论依赖代码、生成输出、runtime
行为或 packaging 时，才运行相应的最小 executable checks。没有实际运行的
检查不能从旧报告复制成“本轮已通过”。
