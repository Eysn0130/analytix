# analytix identity / runtime / settings reset spec

Status: Normative migration decisions with a historical pre-migration audit.
Current as of: 2026-07-10 for the currentness notes only.
Source of truth for as-built behavior: current code, tests, and
`docs/analytix/README.md`.

The frozen identity, bridge, CLI, and migration invariants remain normative.
Sections labelled as current architecture/risk describe the original Kun
baseline unless a dated currentness note explicitly says otherwise.

## 1. 目的

本 spec 定义 analytix（原 Kun）第一阶段源码重构的身份层、runtime 目录、CLI、settings schema、preload bridge、IPC channel、数据迁移边界。

本轮目标不是换皮，而是把原 Kun 项目升级成新的 analytix 架构：

- 用户可见产品名统一为 `Analytix`；包名、可执行文件、CLI、协议、app id、
  environment 和数据目录等 machine identity 保持 lowercase `analytix`。
- `kun/` runtime 目录迁移为 `packages/runtime`。
- CLI 只保留 `analytix serve`。
- settings 从 `agents.kun` 升级为顶层 `runtime`。
- bridge 从 `window.kunGui` 升级为 `window.analytix`。
- IPC channel 从 `kun:*` / 混杂 legacy channel 升级为 `analytix:*` 或清晰业务域 channel。
- 第一阶段不删除现有功能，不精简 Code / Write / Connect Phone / Schedule / UI Plugin / workspace / terminal / git / worktree 等能力。

## 2. 当前架构事实

Currentness note (2026-06-20): 本节记录的是迁移开始前的初始审计状态，
不是当前执行依据。当前源码事实以 `/Users/sun/Projects/analytix/AGENTS.md`、
spec `06` sections `22` / `23`、spec `08` current-state addendum、当前代码与
focused tests 为准：生产 agent core 位于 `packages/runtime-go`；
`packages/runtime` 提供公共 contracts/config/telemetry 与
`analytix serve` launcher。renderer bridge 是 `window.analytix`，
runtime-owned settings 使用顶层 `runtime`，模型 profiles 使用顶层
`provider`，Electron + React + TypeScript 仍是桌面 UI 主权边界。

当前项目仍然是原 Kun/DeepSeek-GUI 架构：

```text
src/main        Electron main process
src/preload     contextBridge + ipcRenderer bridge
src/renderer    React + Zustand renderer
src/shared      cross-layer types/settings/contracts
kun/            bundled runtime package, exposes kun serve over HTTP/SSE
```

当前主数据路径：

```text
Renderer -> window.kunGui -> preload -> ipcMain -> main -> Kun runtime HTTP/SSE -> renderer store -> MessageTimeline
```

关键耦合点：

- `/Users/sun/Projects/analytix/AGENTS.md` 明确当前 runtime 是 bundled `kun/` package。
- `/Users/sun/Projects/analytix/src/preload/index.ts` 暴露 `window.kunGui`。
- `/Users/sun/Projects/analytix/src/shared/app-settings-types.ts` 当前 `AppSettingsV1` 包含 `agents: KunSettingsEnvelopeV1`。
- `/Users/sun/Projects/analytix/src/shared/app-settings-kun.ts` 当前默认读取/写入 `agents.kun`。
- `/Users/sun/Projects/analytix/src/renderer/src/agent/kun-runtime.ts` 当前 runtime provider id/displayName 仍为 `kun` / `Kun`。
- `/Users/sun/Projects/analytix/src/main/runtime-sse-ipc.ts` 当前使用 `kunThreadEventsPath` 和 Kun runtime adapter。
- `/Users/sun/Projects/analytix/electron-builder.config.cjs` 当前 `asarUnpack` / `files` 打包 `kun/dist`。

## 3. 目标命名模型

第一阶段最终源码命名目标：

| 当前 | 目标 | 说明 |
| --- | --- | --- |
| `kun/` | `packages/runtime/` | bundled runtime 源码位置 |
| `kun serve` | `analytix serve` | 唯一 CLI 入口 |
| `build:kun` | `build:runtime` | 顶层 npm script |
| `window.kunGui` | `window.analytix` | renderer preload bridge |
| `KunGuiApi` | `AnalytixApi` | preload API 类型 |
| `agents.kun` | `runtime` | settings 顶层结构 |
| `KunRuntimeSettingsV1` | `AnalytixRuntimeSettingsV1` | shared settings 类型 |
| `KunRuntimeProvider` | `AnalytixRuntimeProvider` | renderer runtime adapter |
| `kun:*` IPC | `analytix:*` IPC | 仅对 runtime/config/session 类 IPC |
| `KUN_READY` | `ANALYTIX_READY` | runtime child ready marker |
| `KUN_*` env | `ANALYTIX_*` env | 不保留 alias |

内部例外：

- `claw` 第一阶段可以保留内部模块名，因为它代表 Connect Phone / IM 能力，不等同 Kun。
- 原 `iKun` / `ikun` 第一阶段需要源码命名改为 `mascot` / `cameo`，素材和模式保留，后期再替换视觉。
- legacy migration 文件可以描述从 Kun 到 analytix 的迁移，但迁移完成后的运行时 schema 不能继续保存 `kun` 命名结构。

## 4. 目标 settings schema

当前：

```ts
type AppSettingsV1 = {
  agents: {
    kun: KunRuntimeSettingsV1
  }
}
```

目标：

```ts
type AppSettingsV2 = {
  runtime: AnalytixRuntimeSettingsV1
  providers: ModelProviderProfileV1[]
  write: WriteSettingsV1
  schedule: ScheduleSettingsV1
  claw: ClawSettingsV1
  // 其他现有 top-level settings 保持语义不变
}
```

迁移规则：

1. 读取旧 settings 时允许一次性读取：

```text
agents.kun -> runtime
```

2. 保存新 settings 时只允许写：

```text
runtime
```

3. 不做双写：

```text
禁止同时写 agents.kun 和 runtime
```

4. 测试中可以保留 legacy fixture，但生产路径不能继续以 `agents.kun` 作为当前状态。

5. `AppSettingsPatch` 同步从：

```ts
{ agents?: { kun?: ... } }
```

升级为：

```ts
{ runtime?: AnalytixRuntimeSettingsPatchV1 }
```

## 5. runtime 目录迁移

目标目录：

```text
packages/runtime/
  package.json
  src/
  tests/
  scripts/
```

顶层引用迁移：

| 当前 | 目标 |
| --- | --- |
| `npm --prefix kun run build` | `npm --prefix packages/runtime run build` |
| `kun/dist/**/*` | `packages/runtime/dist/**/*` |
| `kun/package.json` | `packages/runtime/package.json` |
| `kun/node_modules/**/*` | `packages/runtime/node_modules/**/*` |
| `../../kun/src/...` | `../../packages/runtime/src/...` |

CLI 输出与健康检查：

- ready marker 改为 `ANALYTIX_READY`。
- service 字段改为 `analytix`。
- mode 保持 `serve`。
- HTTP route 语义第一阶段保持兼容，避免 renderer/runtime 双重重写。

示例：

```text
ANALYTIX_READY {"service":"analytix","mode":"serve","port":8899}
```

## 6. preload bridge 目标

当前 preload 是一个较大的 `window.kunGui` 对象，包含 settings、runtime、claw、schedule、workspace、file、write、speech、terminal、update、log 等所有能力。

目标：

```ts
window.analytix = {
  settings,
  runtime,
  connectPhone,
  schedule,
  workspace,
  files,
  write,
  speech,
  terminal,
  updates,
  logs,
  app
}
```

第一阶段可以先保持同一个 bridge 文件，但 API 需要按域分组，避免所有 renderer 代码继续依赖一个巨大的平铺对象。

迁移顺序：

1. 新增 `AnalytixApi` 类型。
2. 新增 `window.analytix` 暴露。
3. renderer 调用从 `window.kunGui.*` 迁移到 `window.analytix.<domain>.*`。
4. 删除 `window.kunGui` 暴露，不保留 alias。
5. 更新测试 fixture。

禁止：

- 不保留 `window.kunGui` fallback。
- 不把 `window.kunGui` 作为 `window.analytix` 的别名。
- 不新增 `window.dsGui` 或任何 DeepSeek legacy bridge。

## 7. IPC channel 目标

保留已有业务域 channel 的语义，但清理 Kun 命名：

| 当前 | 目标 |
| --- | --- |
| `kun:config:read` | `analytix:runtime-config:read` |
| `kun:config:write` | `analytix:runtime-config:write` |
| `kun:config:open-dir` | `analytix:runtime-config:open-dir` |
| `kun:sessions:detect-legacy` | `analytix:sessions:detect-legacy-kun` |
| `kun:sessions:import-legacy` | `analytix:sessions:import-legacy-kun` |
| `runtime:sse:*` | 可保留，因其是 generic runtime domain |
| `runtime:request` | 可保留，因其是 generic runtime domain |
| `claw:*` | 第一阶段可保留内部 channel |

说明：

- `runtime:*` 不含 Kun，可保留。
- `claw:*` 内部保留，但用户可见文案必须为 `Connect Phone`。
- legacy import 可以出现 `legacy-kun`，因为它描述旧数据来源，不是新运行时身份。

## 8. 数据目录

建议：

```text
Electron userData: appData/analytix (independent of visible product-name capitalization)
runtime dataDir: ~/.analytix/data
logs: Electron userData/logs
local traces: Electron userData/traces
crash/error logs: Electron userData/crash
```

迁移策略：

- 新 analytix 默认不写旧 `~/.kun`、`~/.deepseekgui`。
- 如果用户需要导入旧会话，走显式 import 流程。
- 如果需要自动迁移，只做 one-shot migration，并写入 analytix 新目录。

仍需确认：

- 是否把 `~/.analytix/data` 作为最终 runtime dataDir。
- 是否需要自动扫描旧 Kun 数据，还是只提供手动导入。

## 9. 功能保持边界

必须保持：

- Code chat threads。
- HTTP/SSE streaming。
- approvals。
- request_user_input。
- interrupt。
- thread search/archive/fork/resume。
- usage / token / cache 数据。
- workspace picker。
- file read/write/watch。
- git branch/worktree。
- terminal。
- Write workspace、inline completion、export、RAG、PDF/text。
- Connect Phone / claw。
- Schedule tasks。
- UI plugins。
- voice input / speech。
- local logs。
- updater UI。

不允许通过删除功能来降低重构难度。

## 10. 验收标准

源码层：

- `rg -n "Kun|kun|KUN_|deepseek-gui|DeepSeek-GUI|window.kunGui|agents.kun"` 只允许出现在：
  - legacy migration 文件。
  - migration tests/fixtures。
  - 第三方许可或明确历史文档。
  - 用户明确要保留的 mascot 素材原文件名临时清单。
- 生产源码中不再有当前身份意义的 Kun 命名。

运行层：

- `npm run typecheck` 通过。
- runtime build 通过。
- app dev 启动成功。
- `analytix serve` 可启动 HTTP/SSE runtime。
- main process 能启动/停止/restart runtime。
- renderer 能创建 thread、stream reply、interrupt、approve/deny tools。

测试层：

- settings migration tests。
- preload bridge tests。
- IPC schema tests。
- runtime process supervisor tests。
- SSE IPC tests。
- chat store runtime tests。
- packaging config tests。

## 11. 风险

最大风险不是改名本身，而是改名跨越了：

- settings schema。
- preload bridge。
- IPC channel。
- runtime package path。
- tests/fixtures。
- packaging asar rules。
- legacy import。

因此必须分批做，不建议一次性全局替换。

推荐代码阶段顺序：

1. settings schema + migration。
2. runtime directory + CLI。
3. main runtime supervisor。
4. preload bridge。
5. renderer runtime adapter。
6. tests/fixtures。
7. docs/AGENTS 更新。
