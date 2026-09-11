# analytix architecture code upgrade inventory

Status: Historical migration inventory and reference checklist.
Current as of: the initial architecture-reset stage; not current path evidence.
Superseded for as-built navigation by: `AGENTS.md`,
`docs/analytix/README.md`, current code, and later spec currentness addenda.

The target invariants remain useful, but “当前风险” entries are not open work
unless they reproduce against the current worktree. Never recreate an old path
merely because this inventory proposed it.

本清单用于把 analytix（原 Kun）升级为全新 analytix 架构时，必须改造的源码区域、隐患和验收边界列清楚。

原则：

- 不删除、不精简现有功能。
- 不把换皮当架构升级。
- 不在最终源码模块中使用以 Codex 为实现归属的派生命名。
- `CodexDesktop-Rebuild` 只作为 clean-room 行为、布局结构和交互体感参考；没有材料级书面授权时不是代码或资产来源。
- 最终实现命名使用 `Analytix-derived`、`AnalytixThreadVirtualizer`、`AnalytixIconRegistry`、`AnalytixSurfaceTokens` 等 analytix 归属命名。

## 1. identity / release / packaging

当前风险：

- `package.json` 仍包含 `Kun` productName、description、author、homepage、repository。
- `electron-builder.config.cjs` 仍有旧 appId、artifactName、publish prefix、channel、`KUN_*` / `DEEPSEEK_GUI_*` fallback。
- `src/main/app-identity.ts` 仍定义 `APP_PRODUCT_NAME = 'Kun'`，并保留 DeepSeek GUI -> Kun 迁移语义。
- `src/main/index.ts` 仍有旧 `APP_USER_MODEL_ID`。
- `src/main/logger.ts` 仍有 `deepseek-gui` / `kun` log prefix。
- 导出、plugin、updater、菜单、关于页、错误提示中仍可能出现旧产品名。

升级目标：

- app name、shortcut、installer、artifact 全部统一为 `analytix`。
- appId / bundle id / Windows AppUserModelID 统一为 `com.analytix.desktop`。
- artifactName 使用 `analytix-${version}-${os}-${arch}.${ext}`。
- release URL 使用 `https://<release-domain>/analytix/channels/<channel>/latest/`。
- channel 只保留 `stable` / `beta`，默认 `stable`。
- env 前缀统一为 `ANALYTIX_*`。
- author 使用 `Analytix Contributors`。
- repo 暂写 `TBD`，不再指向 `KunAgent/Kun`。

## 2. runtime package / CLI / bundled server

当前风险：

- runtime 位于 `kun/`，GUI 通过 bundled `kun serve` 工作。
- shared types 从 `../../kun/src/contracts/...` 直接导入。
- main process 中存在 `resolve-kun-binary`、`kun-process`、`kun-runtime-supervisor`、`kun-health`、`kun-base-url`、`runtime/kun-adapter` 等命名。
- readiness、stdout marker、error copy、health check、settings 仍围绕 Kun 命名。

升级目标：

- `kun/` 改为 `packages/runtime/`。
- CLI 只保留 `analytix serve`，不保留 `kun` alias。
- runtime stdout marker、health endpoint、error copy、resolver、supervisor 全部改为 analytix。
- runtime contract 从 `packages/runtime/src/contracts` 输出，GUI 不再引用 `../../kun`。
- bundled runtime 作为 analytix 内部 app-server facade 的实现细节，而不是用户可见 `kun serve`。

建议分层：

```text
packages/runtime
  contracts/
  loop/
  services/
  server/
  cli/

src/main/runtime
  analytix-runtime-process.ts
  analytix-runtime-supervisor.ts
  analytix-health.ts
  analytix-base-url.ts
  analytix-app-server-facade.ts
```

## 3. settings schema / data structure / migration

当前风险：

- `AppSettingsV1` 使用 `agents.kun`。
- `KunRuntimeSettingsV1`、`DEFAULT_KUN_DATA_DIR`、`DEFAULT_KUN_MODEL`、`DEFAULT_KUN_PORT` 仍是旧命名。
- 默认目录仍指向 `~/.kun/data`、`~/.kun/write_workspace`。
- provider、workspace、schedule、write、claw 相关设置可能仍从 `agents.kun` 读取。

升级目标：

- settings 从 `agents.kun` 升级为顶层 `runtime`。
- 类型改为 `AnalytixRuntimeSettingsV1`。
- 默认 runtime dataDir 建议为 `~/.analytix/data`。
- write workspace 建议为 `~/.analytix/write_workspace`。
- migration/import 层读取旧来源时使用 `legacy-kun` 语义，生产运行路径不出现旧产品名。

推荐新结构：

```ts
type AppSettingsV2 = {
  runtime: AnalytixRuntimeSettingsV1
  providers: ProviderSettingsV1
  ui: UiSettingsV1
  privacy: PrivacySettingsV1
}
```

## 4. preload / IPC / bridge

当前风险：

- renderer 通过 `window.kunGui` 调用 main。
- shared API 位于 `@shared/kun-gui-api`。
- IPC channel、event name、runtime request name 仍可能包含旧命名。
- 当前 bridge 的领域边界偏宽，chat streaming、settings、runtime control、file/export、permissions 容易混在一起。

升级目标：

- `window.kunGui` 改为 `window.analytix`。
- `@shared/kun-gui-api` 改为 `@shared/analytix-api`。
- IPC 按领域分组：`runtime`、`settings`、`thread`、`files`、`updates`、`permissions`、`diagnostics`。
- 所有 IPC payload 用 typed contract / zod schema 约束。
- bridge 只暴露 stable facade，不把 runtime 细节泄漏到 UI 组件。

## 5. renderer runtime client / provider / contracts

当前风险：

- `src/renderer/src/agent/runtime-client.ts` 直接使用 `window.kunGui`。
- `KunRuntimeProvider`、`dispatchKunRuntimeEvents`、`kun-mapper`、`kun-contract` 等命名和职责混在一起。
- runtime event 到 UI block 的映射还没有形成独立的 projection contract。

升级目标：

- 建立 `src/renderer/src/runtime/analytix-runtime-client.ts`。
- provider 命名改为 `AnalytixRuntimeProvider`。
- mapper 改为 `runtime-event-to-thread-projection` 一类语义命名。
- runtime event、thread projection、UI row model 三层分开。
- Connect Phone / Schedule / Write / SDD / Plan / Preview / Terminal 不直接订阅聊天热路径 store。

## 6. chat hot path / thread smoothness

当前风险：

- `MessageTimeline + useTimelineScroll` 是局部分页，不是真正 measured virtualizer。
- `groupTurns(blocks)` / `deriveTurnSections` 在 render 路径中重复派生。
- streaming text 进入全局 Zustand state，容易牵动非聊天区域。
- Markdown/code block 在 streaming 期间过早重解析、重高亮。
- scroll anchor 依赖 DOM 自然布局和 sentinel，遇到高度变化容易抖动。

升级目标：

- 引入 `Analytix-derived ThreadVirtualizer`。
- 引入 stable `ThreadRow` projection。
- 引入 `streaming-delta-scheduler`，每 frame 最多 flush 一次 text update。
- 引入 `measured-row-cache` 和 batched `ResizeObserver`。
- 引入 bottom-distance anchor，替代反复 `scrollIntoView`。
- finalized 后再执行 rich Markdown / syntax highlight。

推荐目录：

```text
src/renderer/src/thread/
  projection/
  virtualizer/
    analytix-thread-virtualizer.ts
    bottom-distance-anchor.ts
    measured-row-cache.ts
    scroll-layout.ts
  streaming/
  tracing/
```

## 7. visual system / SVG / shadow / border / motion

当前风险：

- CSS 中存在多处局部 `backdrop-filter`、`filter`、复杂 `box-shadow`、`will-change`、transition 规则。
- `base-shell.css`、`markdown-code.css`、`write-editor.css`、`surfaces-write.css` 等文件对滚动、Markdown、写作面板都有性能影响。
- 图标、空状态、启动图、阴影、边框还没有统一 token 和 registry。

升级目标：

- 建立 `AnalytixIconRegistry`。
- 建立 `AnalytixSurfaceTokens`、`ElevationTokens`、`BorderTokens`、`RadiusTokens`、`MotionTokens`。
- 只把可观察的布局、状态和交互写成 Analytix 自有设计/测试；SVG、PNG、WebP、icon 等外部材料只有在书面授权、材料哈希、用途和 notice 已登记后才可进入 `assets/analytix/`。
- 主品牌 logo 以 Downloads 中的 `analytix-logo` 为来源重新导出。
- scroll hot path 不使用重 blur / 重 shadow / 长链 transition。
- 动效优先短、轻、可中断，Windows 优先验证。

## 8. composer / panels / workbench isolation

当前风险：

- Workbench、FloatingComposer、TerminalPanel、DevBrowserPanel、Write/SDD 面板和主线程聊天区共享过多 UI 状态。
- terminal/dev browser/write editor 等重组件可能在聊天 streaming 时被间接触发更新。
- bottom panel、right panel、composer footer 与 timeline scroll reserve 没有形成独立 scroll layout contract。

升级目标：

- 建立 `ComposerController` / `ComposerViewState`。
- bottom panel 与 timeline 使用明确的 scroll reserve 和 sync contract。
- Terminal output 建立自己的 scheduler。
- Dev Browser、Preview、Write、SDD 作为 island 隔离订阅。
- chat streaming 不应驱动非必要 panel render。

## 9. assets / mascot / cameo

当前风险：

- `kun`、`iKun`、旧吉祥物和插画命名混在产品身份中。
- app icon、tray icon、dock icon、installer icon、splash、empty state 没有统一品牌导出链路。
- UI plugin bundle 里仍可能出现旧作者或旧展示名。

升级目标：

- `iKun` 第一阶段改为 `mascot` / `cameo` 命名，素材和模式保留。
- `claw` 内部模块名第一阶段可保留，但用户可见名称统一为 `analytix` / `Connect Phone`。
- app icon 最终输出为 analytix 独立品牌，不叫 Codex，不保留 Kun icon。
- 第一阶段整理 `png` / `ico` / `icns`。
- 建立 asset provenance manifest，记录 source、target、usage、license note、replacement plan。

## 10. logging / tracing / diagnostics

当前风险：

- logger prefix 仍有旧产品名。
- 没有足够本地 tracing 来定位聊天卡顿。
- 远端 telemetry 第一阶段不接入，但本地性能证据必须补齐。

升级目标：

- log prefix 改为 `analytix`。
- crash/error log 放入 analytix userData 目录。
- 本地 tracing 覆盖 runtime event、IPC batch、projection reduce、virtualizer measure、scroll anchor、React commit sample、markdown finalized。
- tracing 输出到 `Electron userData/traces/*.jsonl`，不上传。

## 11. build / bundle / worker

当前风险：

- `electron.vite.config.ts` 目前没有针对 thread、markdown、diff、runtime worker 的明确 chunk/worker 边界。
- Markdown/code/diff 解析如果长期留在主 render path，会限制丝滑上限。
- Electron 本轮不升级，但 bundle 边界仍应先准备。

升级目标：

- 不在本轮升级 Electron。
- 增加 lazy chunks：thread virtualizer、markdown finalization、diff/code heavy parser、write/sdd heavy panels。
- 可引入 dedicated worker：markdown finalization、diff stats、large file parse。
- build 产物中不再出现旧产品名。

## 12. tests / verification

必须补的测试：

- settings migration：`agents.kun` -> `runtime`。
- bridge rename：`window.kunGui` -> `window.analytix`。
- runtime CLI：只接受 `analytix serve`。
- event mapper：runtime event -> thread projection。
- `AnalytixThreadVirtualizer` bottom-distance anchor。
- measured row cache。
- streaming scheduler。
- Markdown finalization queue。
- packaging config snapshot：appId、artifactName、channel、env prefix。
- brand asset manifest。

建议验收：

- `rg -n "\bKun\b|kunGui|agents\\.kun|KUN_|DEEPSEEK_GUI_|deepseek-gui"` 生产源码无非 legacy 例外。
- 100 turns / 500 blocks thread streaming 时 UI 不明显卡顿。
- Windows NSIS x64 能启动、更新配置指向 analytix channel。
- macOS dmg + zip、Linux AppImage x64 至少完成 build smoke test。

## 13. 仍需确认

进入代码重构前仍建议确认：

1. runtime dataDir 是否最终使用 `~/.analytix/data`。
2. 旧数据是否手动导入，还是首次启动 one-shot migration。
3. `legacy-kun` 是否允许只存在于 migration/import/test fixture。
4. 真实 release domain。
5. repo 最终地址。
6. Windows/macOS code signing 策略。
7. 后续是否接入远端 telemetry / Sentry。
8. Electron 升级拆到哪一阶段。

## 14. 推荐执行顺序

```text
Batch 1: identity + release + settings + runtime package
Batch 2: bridge + IPC + runtime provider
Batch 3: brand assets + visual token baseline
Batch 4: Analytix-derived ThreadVirtualizer + streaming scheduler
Batch 5: composer/panel/workbench isolation
Batch 6: worker/lazy chunk + perf tracing hardening
Batch 7: Electron upgrade evaluation
```

最关键的建议：不要先改聊天 virtualizer。先完成 identity、settings、runtime package 和 bridge，否则聊天热路径重构会同时背负旧 Kun 命名、旧数据结构和旧 bridge 的债，风险更高。
