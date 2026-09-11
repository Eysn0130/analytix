## Context

Analytix 已经以 `packages/runtime-go` 作为唯一生产 Agent core，并形成了 Provider 投影、公共事件投影、案件 authority、Host-private carrier 和 typed local display 等敏感数据边界。当前问题不是再建一套“可信运行时”，而是把这些边界收敛为通用 Agent Harness 的内置 Privacy Layer，同时让 Funds 从架构主线退回到首个专业插件。

本设计基于 2026-08-25 对 Codex、Claude Code、DeepSeek Harness 和 OpenCode 官方文档与固定提交的复核，并于 2026-08-26 按最新官方文档和跟踪分支再复核。四者共同证明了稳定 Agent 产品需要一个明确的 Harness 核心以及其上的扩展层；DeepSeek Harness 的 capability seam 和组合模型值得吸收，但“没有特权核心”不适合承担 Analytix 的保密边界。详细证据由 `docs/analytix/upstreams/agent-platform-architecture-recheck-2026-08-25.md` 及其 2026-08-26 addendum 记录。

约束：

- 只有一个生产 Go runtime 和一个 Agent/session/tool/authority 家族。
- Electron + React + TypeScript 继续承担桌面产品面，TypeScript launcher/contracts 不成为第二 Agent runtime。
- 普通 Agent 任务必须在任何专业插件缺失、未授权或故障时继续可用。
- DuckDB、文件和案件源中的敏感原文可以在受保护本地界面显示，但不能因此进入 Provider、普通历史、压缩、记忆、日志、遥测或通用插件通道。
- 目标架构不等于当前 release 已完成；实现和正式保密/发布声明仍由独立验收证据决定。

## Goals / Non-Goals

**Goals:**

- 建立唯一、长期稳定的产品公式：`Go Agent Harness + Plugins + Privacy Layer`。
- 定义 Core、Privacy Layer 和 Plugin 的不可含糊责任边界。
- 让 Coding、Writing、Research、Knowledge、Legal、Funds 等专业能力沿同一插件契约扩展。
- 对模型、子代理、工具、压缩、记忆、持久化、日志、遥测、UI 和 CLI 建立完整敏感数据流规则。
- 用最小可落地路径推进：先统一契约和首方静态注册，再根据隔离需求引入受控的进程外插件，不采用动态 Go `.so`。

**Non-Goals:**

- 不整体迁移到 Codex、Claude Code、DeepSeek Harness 或 OpenCode。
- 不把 Privacy Layer 做成第二 runtime、第二 Agent、第二数据库、第二权限系统或第二 authority。
- 不让“everything is a plugin”扩展到 Provider 出口、权限、持久化投影、插件隔离或本地敏感显示等安全内核。
- 不把 Funds、公安研判或任何单一业务领域固化进 Analytix Core。
- 不在本变更中宣称正式版、商业级保密、合规认证或所有目标能力已经实现。

## Decisions

### 1. 产品只保留一个 Go Agent Harness

产品结构为：

```text
Electron / CLI / API
        |
        v
+-----------------------------------------------------------+
| One Go Agent Harness                                      |
|                                                           |
|  Agent loop · sessions · tools · jobs · subagents         |
|  permissions · approvals · sandbox · provider gateway     |
|                                                           |
|  Built-in Privacy Layer                                   |
|  classify · project · persist safely · audit · display    |
|                                                           |
|  Capability / Plugin Host                                 |
|  Funds · Knowledge · Legal · Research · Writing · Coding  |
+-----------------------------------------------------------+
```

“Harness”“Privacy Layer”“Plugin Host”是同一 Go runtime 内的职责分区，不是三套服务。Agent loop、session、tool lifecycle、subagent、background job、provider 调用、权限和公共事件仍只有一份。

**替代方案：第二条可信 runtime 支线。** 否决。它会复制 session、tool、authority、恢复、日志、升级和测试矩阵，并产生普通任务与敏感任务行为漂移。

**替代方案：TypeScript 主 Harness + Go 敏感 sidecar。** 否决。它恢复双 runtime 选择和双真相源，增加序列化边界与绕过路径。

### 2. Privacy Layer 是不可绕过的 Harness 内核

Privacy Layer 不是一个专业插件。它在所有可能把数据交给模型、持久化、公共输出或扩展代码的出口执行统一策略：

1. **Classify**：为输入、字段、工具结果和派生内容附加数据类别、来源和 authority binding；未知或分类冲突在敏感路径 fail closed。
2. **Project for model**：把原始身份、账号、证件、地址、电话、路径和原始行投影为 host 生成的稳定作用域别名、类型化事实或安全摘要。
3. **Project for durable/public**：普通 session/history、SSE/WS、导出、错误、支持包和遥测只保存/发出允许字段。
4. **Present locally**：UI 或 CLI 如需未脱敏值，只能通过 host-owned typed local presentation broker，从已保留源按当前 principal/case/snapshot/epoch/grant 重读；不得从模型别名或普通历史反向还原。
5. **Audit without content**：记录策略版本、数据类别、digest、计数、decision code 和 authority 状态，不记录原文、反向映射、Provider body 或不受限 metadata。

Core 在最终出口再次执行投影；插件声明“已脱敏”不能跳过 Core 检查。

**替代方案：独立 Token Vault 保存别名到原文的全局反向映射。** 默认否决。原始数据继续由现有受保护 source/DSV2/CAS authority 保存，历史只保留 value-free binding/digest；短期映射限定在当前作用域并可撤销，避免制造第二份高价值持久原文库。若敏感原文来自用户直接输入且没有外部 retained source，需要恢复本机原文时，Host 可把该原始片段保存为同一 private-source authority 下有 retention、principal 和 context binding 的不可变私有输入 artifact；普通 session event 仍只保存安全投影与 value-free display binding。

### 3. 敏感数据使用四种互不替代的投影

| 投影 | 使用者 | 可包含内容 | 禁止内容 |
| --- | --- | --- | --- |
| Private source | Host-owned 数据能力 | 授权范围内的原始 DuckDB/文件值；必要时保存的原始用户输入 artifact | 直接交给普通插件、普通历史或 Provider |
| Model-safe | 主模型、子代理、标题、压缩、memory/embedding/eval | 别名、精确非 PII 事实、受限文本、来源摘要 | 原名、证件号、账号、地址/电话、原始行、路径、反向映射 |
| Public durable | 普通历史、SSE/WS、日志、遥测、导出、支持包 | 公开文本、状态、类型、计数、digest、值无关 authority ref | 原始 Provider/tool body、PII、私有路径、SQL、长 authority 数据 |
| Protected local | Electron UI；受控本地 CLI | 当前授权范围内的原始显示值 | Provider/MCP/通用事件/普通 transcript 的再传播 |

一次数据同时面向多个消费者时必须分别投影，不能复用一个“大而全”的 event/message DTO。UI 可以显示原文，不代表 UI 的通用 store、SSE 或历史可以保存原文。

本地 CLI 的未脱敏输出必须是显式敏感命令：要求当前本地主体授权和交互式 TTY，默认 masked；pipe/redirect、非交互调用、普通日志和 shell history 可见参数路径 fail closed。CLI 与 UI 复用同一个 typed local presentation contract。

### 4. 多轮对话、压缩、记忆和子代理一律视为模型出口

- 用户原始输入先分类并生成 model-safe message；普通模型历史只持久化安全投影、必要语义和 value-free display binding。需要跨重启保留的原始片段进入 private-source authority，由本地 presentation broker 按当前权限解析。
- 工具调用参数、结果和附件在进入主模型、子代理或重试前重新投影；
  Host/app-server/plugin hook 注入的 item 也必须经过同一边界。已持久或
  Host 可注入不等于 model-safe。
- title、summary、compaction、long-term memory、retrieval index、embedding、OCR/vision、guard/eval 和后台模型任务均复用同一 egress policy；它们不是绕过点。
- 子代理只获得父 Harness 为该任务签发的最小能力和 model-safe context，不继承原始 source handle、反向映射或更宽 grant。
- resume/replay 只从受认可的安全历史和 host-private authority 恢复；模型生成摘要不能成为案件事实或显示原文的 authority。

### 5. Plugin 是能力扩展契约，不是任意同进程代码注入

Plugin 明确分为两个不可混同的平面：

- **Package plane**：Analytix-owned canonical package declaration、skills/prompts、
  MCP 声明、hooks、assets 和 public-safe UI 的发现、打包和版本单元；
  Codex/ChatGPT manifest、marketplace/UI metadata 和 MCP metadata 是平台或消费投影。
- **Capability plane**：Go Core 签发、执行和撤销的 typed runtime grant；
  它是敏感数据、Provider、网络、持久化、日志和本地原文显示的唯一授权入口。

安装或加载一个 package 只能让其声明可发现，不能自动获得 runtime
capability。Hook、skill 或 MCP 的输出在作用于 Provider、持久化或公开通道
之前，仍必须经过 Core 的最终授权和投影。

#### 5.1 Analytix-owned canonical package declaration

每个 Analytix distributable plugin package 使用包根下的
`.analytix-plugin/package.json` 作为 package plane 的唯一 canonical source。
该 companion declaration 是 closed、schema-versioned JSON；schema v1 只允许
以下顶层字段：

- `schemaVersion`：整数 `1`；
- `packageId`：稳定的 canonical package identity；
- `packageVersion`：该 package artifact 的 canonical semantic version；
- `contributions`：closed typed declarations，覆盖 skills、MCP servers、hooks、
  assets 和 public-safe UI contribution；
- `requestedCapabilities`：插件请求的 typed Core capability id、protocol 和
  scope constraint；
- `lifecycle`：package 所要求的 lifecycle protocol version 与 entry policy，
  不包含或决定当前 Host state。

所有对象拒绝 unknown field；所有 contribution、entrypoint 和 capability id
在各自作用域内唯一；所有包内路径必须是 normalized package-relative path，
不得越界、使用绝对路径或通过 symlink 逃逸。`packageId`、`packageVersion`、
protocol、entry policy 或路径无效时，package admission 在产生投影、安装状态
或 capability effect 前 fail closed。

`.codex-plugin/plugin.json` 保持 OpenAI/Codex/ChatGPT 的稳定 identity、component
与 install-surface manifest，不承载 Analytix Core capability request/grant 或
未文档化 Analytix 扩展字段。其 identity/version/component metadata，以及
marketplace/UI metadata、`.mcp.json`、MCP server identity/version metadata，
必须从 canonical declaration 确定性派生，或由 executable parity gate 与
canonical declaration 做 exact equality 验证；它们都不是独立版本 owner。

这个选择吸收 OpenAI 官方 plugin packaging 对稳定平台 manifest 与 component
组合的定义；吸收 `obra/superpowers` Codex portal packaging 的 commit-based
deterministic staging、metadata completeness、entry count 和 SHA-256 收据；但不把
任一 harness-specific manifest 提升为 Analytix 多消费方的单一 authority。
tracking-branch 上多个 harness manifest 在任一时点相等或不同，都不能替代
Analytix 自己的 canonical declaration 和 executable parity evidence。

#### 5.2 Host admission policy 与 package version 分离

Go Host-owned static admission policy 独立于 canonical declaration。它定义可接受
的 declaration schema/lifecycle protocol、first-party package identity、entry
policy、capability ceiling、artifact integrity、provenance、签名与 admission
要求。插件 declaration 只能提出请求；它不能生成、扩大或持久化 grant，也不能
把 platform manifest、install marker、marketplace state、MCP metadata 或
manifest presence 变成 authority。

exact `packageVersion` 从 admitted canonical declaration 进入 Go-signed
receipt/index/ready binding，再由 TypeScript 和 MCP startup 消费 typed
receipt/projection。Host policy 不复制发布者 exact version literal；只有具有独立
兼容性理由并由测试固定的显式 version range/deny rule 可以成为 policy gate。
admission 必须按 declaration strict parse、artifact/provenance verification、Host
policy check、signed binding emission 的顺序执行。

#### 5.3 Package lifecycle protocol 与 Host state machine 分层

task 2.1 只定义 declaration 中的 lifecycle protocol version、entry policy、
派生关系和 admission prerequisites。插件不能声明自身 `ready`、`granted` 或
`healthy`。task 2.2 才实现 Host-owned setup/ready/failed/stopped、grant、health、
disable、revoke 和 deterministic recovery state machine。Airi 固定提交中
requested/granted 分离与 Host-owned setup phases 可作为概念参考，但不复制其
代码、默认 grant 行为或同进程信任模型。

#### 5.4 Funds legacy-v0 bounded migration

当前尚无 companion declaration 的 Funds package 只可由显式 `legacy-v0`
adapter 读取。adapter 仅在既有 first-party Host policy、exact packaged artifact
与 Go-signed materialization receipt/index 同时有效时，生成内存中的 v1 typed
declaration/projection；它不回写 package、不信任 user-writable install marker，
也不放宽 capability ceiling。加入有效 v1 companion 后，Funds 必须走 canonical
路径；缺失、unknown、duplicate、invalid 或不兼容 declaration 只禁用 Funds，
健康 Core 下普通 Agent 继续可用。该迁移不开放第三方 executable plugin；后者
仍等待 task 2.5 的隔离合同。

#### 5.5 Local-nonpublishable native execution admission

First Runnable 的 source-bound development 复用现有
`nativecomponentregistry.AuthorityUseLocalBuild`、receipt/registry、platform
verifier 和 native owner 抽象，不创建第二个 registry、Agent Core、build 或
publication authority。该 admission 与 release authority 是两个不可升级的用途：

- local-build 仅接受 exact `development_non_publishable` package
  classification、fresh isolated profile、synthetic/fixture source、当前 source
  closure、toolchain、target、component 与 package/runtime identity hashes；
- local-build platform policy 只接受 exact ad-hoc signature，拒绝 Developer ID、
  Apple team identity、notary、upload、promote、publish 或 release intent；
- package builder 只可嵌入当前 generation 的 exact local-build binding；runtime
  Go Host 仍独立重验 Host policy、package classification、generation receipt、
  registry inventory、platform policy 与当前 isolated-profile authority；
- declaration、`requestedCapabilities`、manifest、marker、package presence、环境
  变量、CLI 参数或普通配置都不能铸造 native owner 或 typed plugin grant；
- local-build authority 是 process-local、不可序列化、不可导出且可随 owner
  关闭而失效；它不能进入 live user data、普通 profile、正式 package 或发布链。

正式包保持唯一既有路径：`controlled_release_receipt`、Developer ID/platform
signature 与 exact packaged identity 全部成立后，release-mode owner 才可打开。
local-build evidence 只能支持 focused development acceptance，不能形成 Package、
Product、Formal RC、notary 或 release 声明。

**替代方案：把 development marker 视为 release receipt。** 否决。marker 与
package presence 只是 evidence，不能自授 authority，也不能从 local-build 提升为
`controlled_release`。

**替代方案：新增 Cargo execution/publication authority family。** 否决。
First Runnable 复用当前 registry/verifier/owner seam；不新增
`CargoExecutionReceiptV1`、`PublicationPermitV1` 或第二 publication authority。

**替代方案：向 `.codex-plugin/plugin.json` 增加 Analytix 私有字段。** 否决。
该文件属于 Codex/ChatGPT install surface；依赖未文档化字段会把 Analytix Core
合同绑定到外部平台解析和兼容策略。

**替代方案：让 Codex、Claude、marketplace、MCP 或 Go 常量各自维护 package
version。** 否决。平台适配需要保留，但独立 release-version owner 会把一次升级
变成多点手工同步，并使 fail-closed 漂移表现为 Funds outage。

**替代方案：从插件 declaration 自动生成 Host grant。** 否决。declaration 是
不可信请求；Host policy、当前 authority 和 typed grant 必须独立决定许可范围。

插件包可以贡献：

- 专业工具、工作流、skills/prompts 和 domain schema；
- domain classifier/canonicalizer 的候选规则，但不能降低 Core 分类；
- provider/public-safe UI extension 与展示 schema；未脱敏值只能请求 Core-owned protected local component 显示，任意插件 renderer 不接收原文；
- 通过 canonical package declaration 请求的 model/provider/tool/data capabilities；
- 由 Host 注入的 typed service handle。

插件不得直接获得：

- DuckDB 路径、任意 SQL、原始 DB handle、Provider key 或底层 socket；
- 无范围文件系统/shell/network；
- 原始普通历史、Host-private carrier、反向映射或 authority 实现；
- 绕过 Core 的 Provider、持久化、日志、遥测、SSE/WS 或本地显示出口。

插件信任分级：

1. **Core extension**：仓库内 Go 代码，只有安全内核职责可以进入；不按业务插件发布。
2. **First-party professional plugin**：manifest、skills、UI 与业务能力属于插件；需要敏感本地计算时，由编译/打包固定身份的 Go native capability 或受控进程提供 typed handle。v1 使用静态注册/固定打包，不加载动态 Go `.so`。
3. **Third-party plugin**：默认只获得低风险能力；需要执行代码时采用进程外隔离、显式 capability 和可撤销授权。第三方插件默认不能读取敏感原文。

插件故障、缺失、授权撤销或必需隔离不可用只关闭对应 capability，
不能静默降级为 unsandboxed 执行，也不阻塞健康 Core 下的普通 Agent runtime
启动和日常任务。

### 6. Funds 是首个旗舰专业插件

Funds 用于证明平台能在保持普通 Agent 体验的同时进入资金分析、公安情报研判和其他高敏感专业工作。它可包含资金领域 schema、工具、查询计划、证据工作流、技能、UI 和报告模板；通用 Agent loop、权限、Provider gateway、Privacy Layer、session、publication 和 plugin lifecycle 不属于 Funds。

现有 case/evidence/claim/final-gate 实现可以作为 Funds 及未来高敏插件使用的通用受保护 capabilities 逐步抽取，但不能通过改名把资金语义留在 Core。当前共享 Go composition 中仍有 Funds-specific 工具名、logical effect、输出类型和 local reader；它们必须迁移到注册式 typed capability，同时保留历史事件、工具名和 replay compatibility。普通启动路径也必须逐步解除对可选案件/evidence store 的无条件依赖。

### 7. 上游只吸收机制，不外包产品 authority

- **Codex**：吸收 Harness/Host 分工、工具循环、sandbox/approval、plugin
  manifest/package 与 skills/MCP/hooks/assets 组合，但不把包加载视为敏感能力授权。
- **Claude Code**：吸收稳定 built-in tool foundation、permission/sandbox、skills/hooks/MCP/subagents 的扩展方式。
- **DeepSeek Harness**：吸收 capability seam、profile/bundle composition、append-only session projection、工具流水线、continuable subagent；不吸收“无特权核心”和默认完整 telemetry payload。
- **OpenCode**：吸收显式 model-message lowering、session event/compaction 结构、plugin lifecycle 和权限体验；不吸收插件取得 raw history/shell/SDK、raw history 直接进入模型、或 permission 替代数据投影。

任何代码、prompt、skill 或资产吸收仍需固定提交、许可证/provenance 审查和可验证的 Analytix 落点。

### 8. 品牌表述与工程完成度分开

GitHub 和产品入口可以使用既定品牌句与目标架构，但必须链接到 currentness 说明。`Private by design` 是技术品牌与设计原则，不自动构成认证、合规或 release-ready 声明。正式发布仍需当前 commit/package 上的普通任务、敏感任务、Provider、恢复、Electron、CLI、插件隔离和泄漏 canary 证据。

## Risks / Trade-offs

- **[Risk] 插件化范围过大，重新引入复杂度。** → v1 只定义一个 canonical package declaration/parity 家族；Host capability 与 lifecycle state machine 分别由 task 2.2 承担，首方专业插件允许静态注册，动态分发按真实需求后置。
- **[Risk] 同进程首方插件技术上能访问进程内存。** → 敏感原文处理限定为审计过的 Host-owned native capability；第三方可执行插件必须进程外隔离，不能把同进程接口称为安全沙箱。
- **[Risk] 安装成功被误读为已授权。** → package admission 与 runtime capability
  grant 分开建模、分开事件和分开撤销；隔离不可用时关闭插件 lane。
- **[Risk] Privacy Layer 变成散落的脱敏 helper。** → 所有模型、持久化、公共输出和本地显示出口由 Core 的集中 policy engine 和正向 schema 投影封口，并以 hostile runtime canary 验证。
- **[Risk] 别名导致多轮语义断裂。** → 别名绑定 principal/case/snapshot/context epoch，使用 host-verifiable value-free binding；跨作用域不复用，恢复时重新验证。
- **[Risk] UI 显示原文导致通用 store、调试工具或截图泄漏。** → 原文走专用 typed sink、短租约和 no-store 路径；通用 renderer state/事件仍只持有安全投影，并明确残余的屏幕/操作系统风险。
- **[Risk] 可选案件设施阻塞普通启动。** → 把专业 store inventory/recovery 移入插件 capability 初始化；失败只禁用对应插件，安全内核/持久化本身的物理损坏仍按其影响范围 fail closed。
- **[Risk] local-build evidence 被误升格为 release trust。** → 将
  `AuthorityUseLocalBuild`、`development_non_publishable`、ad-hoc policy、isolated
  profile 与 synthetic source 绑定为一个不可升级的 Host decision；所有
  Developer ID/notary/upload/promote/publish 入口显式拒绝该用途。
- **[Trade-off] Core 比 DeepSeek Harness 的“everything is a plugin”更有特权。** → 这是敏感数据产品的必要安全边界；可替换性让位于可证明的不可绕过性。

## Migration Plan

1. **品牌与契约**：落地本变更、accepted spec、编号产品规范、README 和插件定位；旧文档保留历史内容但由新规范在同 scope 内 supersede。
2. **统一插件契约**：先落地 `.analytix-plugin/package.json` schema v1、确定性平台投影、Host admission、签名 binding 与 Funds `legacy-v0` adapter；再由 task 2.2 落地分离的 Core-owned grant/health/disable/revoke/recovery state machine，并以 task 2.2a 接通不可升级的 local-nonpublishable native execution admission。
3. **Privacy Layer 封口**：建立统一数据分类与四投影 schema，逐一覆盖主模型、子代理、工具、title、compaction、memory、embedding、日志、遥测、SSE/WS、导出和 local display。
4. **Funds 迁移**：以首方旗舰插件注册现有 Funds 能力；将可复用 case/evidence/publication capability 归入 Core，将资金 schema/query/workflow/UI 留在插件。
5. **普通路径解耦**：让专业插件 inventory/recovery/semantic fault 只禁用其 capability，验证普通 Coding/Writing/Research 在 Funds 不存在或损坏时仍工作。
6. **插件隔离**：先完成静态首方插件；再为第三方可执行插件加入进程外 Host、最小 capability、资源限额、网络/文件策略和撤销。
7. **正式验收**：以同一 release artifact 运行 ordinary、sensitive、resume/replay、subagent、compaction、plugin-fault、provider、UI/CLI local-display 和零泄漏 canary；只有证据闭合后提升 release/保密声明。

回滚策略：文档和 capability registration 可回退到上一个已接受版本；实现迁移必须保持现有唯一 Go runtime 和现有持久数据可读，不通过创建兼容性第二 runtime 回滚。local-build admission 可通过移除其 composition/binding 回退为 native capability unavailable，默认 release admission 与正式包信任不变；专业插件迁移失败时禁用该插件并保留普通 Agent 路径。

## Open Questions

- 第三方高敏插件的首个执行隔离载体最终选择受控子进程、WASI/Component Model，还是两者分层，需要在具体插件和跨平台 benchmark 后决定。
- 本地 CLI 未脱敏显示的 TTY、copy/export 与操作系统审计策略需要单独 OpenSpec；在此之前不扩大现有 typed local display 权限。
- 哪些现有案件/evidence components 应提升为通用 Core protected capabilities，需按“是否包含业务语义”逐项审计，不做批量搬迁。
