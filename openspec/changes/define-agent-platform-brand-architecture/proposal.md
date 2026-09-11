## Delivery and dependency scope

This change defines a platform target, not a claim that its complete roadmap is
already implemented or a prerequisite for every deliverable. Shared work with
`case-evidence-publication-gate` (plugin failure isolation, privacy projections
and Funds startup separation) uses the same implementation and appropriately
scoped evidence. Preserve each change's acceptance boundary; do not implement
or rerun the same obligation solely because both task lists mention it.

The case change's A0/B1 convergence map defines its first-stage finish line.
Third-party executable isolation is required before admitting those plugins;
it does not require enabling them to deliver an already scoped first-party
milestone. All open platform tasks remain open for this change's own completion.

The [2026-09-10 recovery checkpoint](../../../docs/analytix/handovers/2026-09-10-owner-replacement.md)
records the current shared R131 candidate and unresolved terminal-persistence
finding. Tasks 2.4, 4.3 and 4.4 are still open; old focused ledger entries and
the writer's safe retirement do not close them. Use the case change's current
checkpoint and the preserved candidate rather than creating a second plugin
isolation implementation. This routing update changes no accepted requirement,
task checkbox, or third-party admission boundary.

## Why

Analytix 的公开入口仍主要定位为桌面 Agent 工作台，而产品与规划材料又分散使用 Agent OS、Funds 和多层技术名词，无法稳定指导产品、文档和后续架构迭代。现在需要把品牌、唯一 Go Agent Harness、插件扩展和不可绕过的隐私边界收敛为一个可执行的长期产品契约。

## What Changes

- 将品牌类别统一为 **Agent Platform**，采用 `Analytix — Agents for sensitive work.` 及其指定中英文品牌表达。
- 将面向外部的架构表达统一为 **Go Agent Harness + Plugins + Privacy Layer**。
- 明确只有一个生产 Go Agent Harness；Privacy Layer 是该 Harness 内置且不可绕过的横切边界，不是第二运行时、第二权限系统或可选插件。
- 建立 Core 与 Plugin 的稳定责任边界：Core 拥有 Agent 循环、权限与隔离、模型出口、会话与公共投影、插件能力授予和本地受保护显示；Plugin 只通过类型化 capability 扩展专业工具、工作流、知识、提示和 provider/public-safe UI。
- 为 source-bound development 增加独立且严格受限的 `local-nonpublishable` native execution admission：复用现有 Go registry/verifier/owner 抽象与 `AuthorityUseLocalBuild`，但不创建 release、publication 或第二 authority family。
- 将 Funds 定义为首个旗舰专业插件，不再作为 Analytix 主品牌、主产品定义或 Core 架构的一部分；Knowledge、Legal、Research、Writing、Coding 等后续能力沿同一插件模型扩展。
- 规定敏感数据在 Provider、子代理、压缩、记忆、持久化、日志、遥测和本地显示中的统一流转边界，并禁止插件绕过这些边界。
- 在 GitHub 入口 README 和规范索引中同步品牌与架构方向，同时将当前已实现状态和未来目标明确分开。

## Capabilities

### New Capabilities

- `agent-platform-foundation`: 定义 Analytix Agent Platform 的品牌契约、唯一 Go Agent Harness、内置 Privacy Layer、插件能力边界、敏感数据流和旗舰 Funds 插件定位。

### Modified Capabilities

<!-- None. Existing case capabilities remain valid implementation contracts under the new platform foundation. -->

## Impact

- 公开产品入口：`README.md`、`README.en.md`、`plugins/README.md`。
- 规范与架构文档：`docs/analytix/`、`docs/analytix/specs/` 和新增的 accepted capability spec。
- 后续实现：`packages/runtime-go` 的插件生命周期、能力授予、Privacy Layer 投影，以及 Electron/CLI 的本地受保护显示边界。
- 兼容性：不更换现有唯一 Go 生产 runtime，不创建第二 Agent、第二会话系统、第二权限系统或第二敏感数据 authority；现有 Funds/案件能力按插件 capability 逐步收敛。
- 上游策略：Codex、Claude Code、DeepSeek Harness、OpenCode 仅作为固定版本研究输入；吸收机制前仍须遵守上游许可、provenance 和 admission 流程。
