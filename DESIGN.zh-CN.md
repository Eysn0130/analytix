---
# DESIGN.zh-CN.md frontmatter -- 面向设计代理的机器可读设计上下文。
# 这里指向真实代码位置，不重复复制所有 token。

schema_version: 2
project: analytix
product_name: Analytix
product_category: Agent Platform
brand_line: Analytix — Agents for sensitive work.
architecture: Go Agent Harness + Plugins + Privacy Layer
runtime_core: packages/runtime-go
runtime_launcher_contracts: packages/runtime
runtime_cli: analytix serve
preload_bridge: window.analytix
settings_schema:
  runtime: runtime-owned desktop settings
  provider: model provider profiles
themes: [light, dark, system]
brand_assets:
  root: src/asset/brand
  provenance: src/asset/brand/provenance.json
  app_icon: src/asset/brand/analytix-app-icon-512.png
  splash: src/asset/brand/analytix-splash.png
visual_tokens:
  css: src/renderer/src/styles
  tokens: src/renderer/src/design/analytix-visual-tokens.ts
  icon_registry: src/renderer/src/design/AnalytixIconRegistry.ts
thread_runtime:
  projection: src/renderer/src/thread/projection
  virtualizer: src/renderer/src/thread/virtualizer
  streaming: src/renderer/src/thread/streaming
  tracing: src/renderer/src/thread/tracing
---

# Analytix 设计指南

**Analytix — Agents for sensitive work.** Analytix 是胜任日常、更胜任敏感业务的 Agent Platform。设计改造必须保留现有 coding、写作、计划、审查、自动化和专业插件产品面，并以 `docs/analytix/specs/` 登记的规范约束产品身份、runtime 架构、品牌资产、隐私边界和聊天体感。先从 `docs/analytix/README.md` 区分 accepted target、当前实现与定期证据。

## 产品形态

- 用户可见品牌是 **Analytix**，产品类别是 **Agent Platform**。package、
  executable、CLI、protocol、app 和环境变量等机器身份按既有 contract 继续使用
  lowercase `analytix`。
- 架构表达是 **Go Agent Harness + Plugins + Privacy Layer**；三者属于同一个
  Go runtime，不是彼此独立的 runtime。
- 唯一生产 Agent core 是 `packages/runtime-go` 中的 Go runtime。
- `packages/runtime` 包含 TypeScript launcher、公开 contracts、config 与 telemetry；其 `analytix serve` CLI 启动 Go `runtime-server`。
- renderer 通过 `window.analytix` 访问 preload bridge。
- runtime-owned 桌面设置使用顶层 `runtime`，模型 Provider profile 使用顶层 `provider`。
- runtime 环境变量使用 `ANALYTIX_*` 前缀。
- 普通启动使用本地 Provider 引导与 Registry 就绪状态；Settings 通过受保护 Secret Store 管理 Provider 连接。Hub 仅保留显式延迟加载的兼容入口，不是启动依赖。
- Official Standard Windows 的地区化显示名为 `Analytix灵鉴`，当前产物为 `analytix-standard-${version}-x64.exe`；其可执行文件、app id、CLI 和环境变量身份仍为 `analytix`、`com.analytix.desktop`、`analytix serve` 与 `ANALYTIX_*`。
- 手机/IM 工作流的用户可见名称是 `Connect Phone`。
- Funds 是首个旗舰专业插件，不是主品牌或 Core。Knowledge、Legal、Research、
  Writing、Coding 等后续专业能力沿同一个 plugin 方向扩展。
- 未脱敏敏感值只通过 Core-owned protected local presentation surface 显示；
  generic renderer state 与任意 plugin UI 只能接收允许的 public/model-safe
  projection。
- 保留的形象模式在当前源码和 UI 中使用 mascot/cameo 命名。

## UX 原则

- 保留现有用户流程、入口和显示语义。
- 核心工作台保持克制、信息密度高、面向重复工作的桌面工具感。
- 不要在改身份、资产、runtime contract 或聊天虚拟化时隐藏功能。
- 紧凑命令优先使用图标，模式使用分段控件，二元设置使用 toggle，数值设置使用 slider/input，选项集合使用 menu。
- 重复卡片半径保持 8px 或更小，除非现有组件明确需要更大值。
- 聊天 streaming 热路径避免重 blur、重 shadow 和触发布局的动画。

## 视觉系统

真实视觉来源在代码中：

- `src/renderer/src/styles/*.css`：shell、组件和主题变量。
- `src/renderer/src/design/analytix-visual-tokens.ts`：surface、elevation、border、radius 和 motion 语义 token。
- `src/renderer/src/design/AnalytixIconRegistry.ts`：应用图标注册。
- `src/asset/brand/provenance.json`：品牌资产来源记录。
- `build/icon.icns` 与 `build/icon.ico`：打包图标。

品牌资产应保留在 `src/asset/brand` 下，不暴露参考来源产品身份。新增视觉资产如果属于产品源码，需要记录 provenance；本地构建输出不要提交。

## Workbench 布局

桌面 shell 由 Electron main process 与 React renderer 组成：

```text
Electron main
  -> preload bridge: window.analytix
  -> React workbench
  -> local HTTP/SSE runtime: packages/runtime-go (Go runtime-server)

analytix serve: packages/runtime (TypeScript launcher/contracts)
  -> packages/runtime-go (Go runtime-server)
```

Code、Write、SDD、Preview、Dev Browser、Terminal、right panel、bottom panel 和 Connect Phone 是 workbench islands。它们应该订阅自身表面所需的最小状态，不能把 token-by-token chat streaming 放大成整应用重渲染。

## Chat Timeline

生产聊天路径是：

```text
runtime event
  -> thread projection
  -> stable row model
  -> Analytix-derived ThreadVirtualizer
  -> row renderer
```

约束：

- 不要在 render 热路径重复派生 turn sections。
- streaming text delta 先进入 buffer，每帧最多 flush 一次。
- 结构事件可以及时 flush，保持工具调用、审批和 process 状态响应。
- measurement 使用 row cache 与 batched `ResizeObserver`。
- 用户接近底部时保持 bottom distance。
- 用户阅读历史时不要被新 token 拉回底部。
- history prepend 必须保持 first visible row 的视觉位置。
- streaming markdown/code 使用 lightweight text surface；finalized 后再渲染 rich markdown 和 syntax highlight。

## Runtime 与数据

默认本地数据位置：

```text
~/.analytix/data
~/.analytix/write_workspace
```

第一阶段旧数据策略是显式 import/migration。当前 UI、runtime launch、packaging、assets、release metadata 和 settings 不应使用旧产品身份。

SDD 需求位于 `.analytixsdd/requirements/<uuid>/requirement.md`，GUI plan 直接位于 `.analytixsdd/plan/`。

## 验证

闭环级设计或架构工作需要运行：

```bash
npm run test
npm run typecheck
npm run build
npm run lint
git diff --check
```

同时运行 `docs/analytix/specs/06-implementation-closure-and-acceptance.md` 中的命名扫描，并把剩余命中分类为 current-product bug、legacy-kun migration/import/test fixture allowed、provider/model name allowed、historical doc isolated 或 third-party/vendor path allowed。

Go runtime 改动需要 Go 1.22+ 兼容工具链、`gofmt` 与聚焦测试，例如：

```bash
(cd packages/runtime-go && go test ./...)
```
