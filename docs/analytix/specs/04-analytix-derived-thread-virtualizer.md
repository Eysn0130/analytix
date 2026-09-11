# Analytix-derived ThreadVirtualizer / chat smoothness spec

Status: Normative scrolling/streaming invariants with historical implementation
inventory.
Current as of: 2026-07-10 for this lifecycle note.
Source of truth for as-built wiring: current renderer code, focused tests, and
fresh real-window evidence.

Sections that mention `KunRuntimeProvider`, `dispatchKunRuntimeEvents`, or an
unvirtualized `MessageTimeline` are pre-implementation audit records. Preserve
them as migration context; do not use them as current path guidance.

## 1. 目的

本 spec 维护 analytix 聊天中间区已经落地的 projection、streaming
scheduler、measured-row cache 与 Analytix-native virtualizer 不变量；历史
`MessageTimeline + useTimelineScroll` 链路只保留为迁移基线。
`/Users/sun/Projects/_upstreams/CodexDesktop-Rebuild` 只作为可观察行为、
滚动结构和体感参考来源，不是代码或资产复用授权。

目标：

- 保留现有消息功能和样式语义。
- 引入 Analytix-derived bottom-distance virtualizer / scroll layout。
- 降低 streaming token、Markdown/code block、tool block、history prepend 对 React commit 和 DOM reflow 的压力。
- 第一阶段不升级 Electron，不引入 Rust 重写。

## 2. 命名边界

最终落到 analytix 源码、模块、类型、目录和资产 manifest 中的命名，必须体现 analytix 自己的实现归属：

- 使用 `Analytix-derived ThreadVirtualizer`、`AnalytixThreadVirtualizer`、`ThreadScrollLayout`。
- 推荐目录为 `src/renderer/src/thread/virtualizer/analytix-thread-virtualizer.ts`。
- 资产输出目录使用 `assets/analytix-derived/` 或 `assets/analytix/`。
- 文档只有在描述参考来源时才写 `CodexDesktop-Rebuild` 或 `Codex reference`。
- 禁止最终源码中出现以 Codex 为实现归属的派生命名、旧派生资产目录或旧派生 ThreadVirtualizer 命名。

这条规则的含义是：可以从公开/可观察行为独立定义 bottom-distance
virtualizer / scroll layout 的需求和验收测试，再以 clean-room 方式实现
Analytix 模块。固定提交没有覆盖重建脚本、下载应用和本机提取物的项目
许可证；未登记材料级书面授权前，不移植代码片段或资产。

## 3. 迁移前 analytix 聊天架构（历史）

当前关键路径：

```text
runtime SSE
  -> src/main/runtime-sse-ipc.ts
  -> preload runtime:sse-event
  -> rendererRuntimeClient.onSseEvent
  -> KunRuntimeProvider.subscribeThreadEvents
  -> dispatchKunRuntimeEvents
  -> buildThreadEventSink
  -> Zustand chat store
  -> MessageTimeline
  -> useTimelineScroll
```

已有优化：

- main process 已按 network chunk 批量转发 SSE events。
- `dispatchKunRuntimeEvents` 已把连续 text/reasoning delta 合并到 `sink.onDeltas`。
- `buildThreadEventSink` 已基于 batch 更新 `liveAssistant` / `liveReasoning`。
- `useTimelineScroll` 已避免流式时展开全部历史，且支持隐藏旧 turns。

当前瓶颈：

1. 不是 measured virtualizer
   - `MessageTimeline` 仍然渲染 visible turns 对应的真实 DOM。
   - 旧历史只是按 turn count 隐藏，不是按像素测量和 viewport window 虚拟化。

2. `groupTurns(blocks)` 每次 render 重新派生
   - streaming 时 `live.length` 变化会驱动 timeline 重新计算。
   - 长线程中 blocks/turns 派生会越来越贵。

3. live text 存在全局 Zustand state
   - 每批 delta 都会更新 store。
   - 依赖 chat store 的组件可能被动收到更新。
   - 需要用 projection store 和更窄 selector 隔离热路径。

4. scroll anchor 依赖 DOM 自然布局
   - 当前使用 `scrollIntoView` 和 `scrollHeight - scrollTop - clientHeight`。
   - Markdown/code block 渲染后高度变化会导致二次修正。
   - 大代码块、图片、review/tool cards、generated files 会造成测量抖动。

5. Markdown/code block 渲染热
   - `StreamdownAssistant`、`StreamdownCode`、code collapse/ResizeObserver 会在 streaming 时持续参与渲染。
   - 需要分离 raw streaming text 与 finalized rich markdown。

## 4. CodexDesktop-Rebuild 对照结论

`/Users/sun/Projects/_upstreams/CodexDesktop-Rebuild` 是 Codex App 的
重建/提取流水线，不是完整未编译源码。其被忽略的 `src/*` 本机提取物
只能支持临时行为观察，不能作为 commit-pinned 源码或授权证据。可观察到：

- Electron 42 + Electron Forge + Vite plugin/Fuses。
- `app-server-types` / `protocol` / `commands` / `shared-node` 等 workspace 依赖，说明 app/server/protocol 边界更强。
- webview assets 按 thread shell、composer、markdown surface、app-server manager、side panel、bottom panel、worker 等细粒度 chunks 拆分。
- 存在 `runtime.worker-*.js`、`markdown-surface-*.js`、`composer-*.js`、`thread-app-shell-chrome-*.js` 等独立 bundle。
- 包内存在 trace recording 文案和 product logger 相关模块。
- 视觉层有 `scroll-to-bottom-button`、`app-shell-bottom-panel-scroll-sync`、`right-panel-composer-overlay-scroll-reserve` 等滚动/布局相关模块。

结论：

- Codex 的丝滑感不是 Electron 单点带来的。
- 更像是 protocol/app-server 边界、thread projection、worker/chunk 拆分、scroll layout、trace tooling、视觉约束共同作用。
- analytix 应把可验证的 bottom-distance 行为写成自己的不变量、测试和
  benchmark，再独立实现 Analytix-derived ThreadVirtualizer；不复制
  CodexDesktop-Rebuild 的提取片段或 app-server 实现。

## 5. 已落地架构与持续目标

当前 tracked renderer 模块包括：

```text
src/renderer/src/thread/
  projection/
    thread-projection-store.ts
    thread-row-model.ts
    thread-event-reducer.ts
    thread-selectors.ts
  virtualizer/
    analytix-thread-virtualizer.ts
    bottom-distance-anchor.ts
    measured-row-cache.ts
    scroll-layout.ts
    resize-observer-batcher.ts
  streaming/
    streaming-delta-scheduler.ts
    streaming-text-buffer.ts
    markdown-finalization-queue.ts
  tracing/
    thread-performance-trace.ts
```

React 组件目标：

```text
components/chat/MessageTimeline.tsx
  -> components/chat/ThreadTimeline.tsx
  -> thread/virtualizer/AnalytixThreadVirtualizer
  -> row renderers
```

保留现有 row 视觉组件：

- `MessageBubble`
- `GeneratedFilesPanel`
- `ReviewPlanCard`
- `ReviewSummaryCard`
- `TurnChangeSummary`
- `WorkMetaRow`
- `ProcessSectionRow`
- `MessageTimelineEmptyHero`

这些不删除，只是从“直接 map turns 渲染”迁移为“virtual row renderers”。

## 6. row model

当前 timeline 按 `Turn` 渲染，目标改成 stable row model：

```ts
type ThreadRow =
  | { kind: 'emptyHero'; id: string }
  | { kind: 'forkBanner'; id: string }
  | { kind: 'loadEarlier'; id: string; hiddenCount: number }
  | { kind: 'turnHeader'; id: string; turnId: string }
  | { kind: 'user'; id: string; blockId: string; turnId: string }
  | { kind: 'assistant'; id: string; blockId: string; turnId: string; streaming?: boolean }
  | { kind: 'reasoning'; id: string; blockId: string; turnId: string; streaming?: boolean }
  | { kind: 'tool'; id: string; blockId: string; turnId: string }
  | { kind: 'review'; id: string; blockId: string; turnId: string }
  | { kind: 'approval'; id: string; blockId: string; turnId: string }
  | { kind: 'userInput'; id: string; blockId: string; turnId: string }
  | { kind: 'generatedFiles'; id: string; turnId: string }
  | { kind: 'liveProgress'; id: string; turnId: string }
  | { kind: 'bottomSpacer'; id: string }
```

原则：

- 每个 row 有 stable id。
- row data 是 render-ready projection，不在组件内重复 `groupTurns`。
- streaming row 只更新对应 row 的 text buffer。
- finalized row 和 streaming row 的渲染策略不同。

## 7. bottom-distance anchoring

当前 stick-to-bottom：

```text
distanceToBottom = scrollHeight - scrollTop - clientHeight
scrollIntoView(end)
```

目标：

```text
bottomDistance = totalMeasuredHeight - scrollTop - viewportHeight
```

当用户接近底部：

- 新 delta 到来时保持 bottomDistance 约等于 0。
- row 高度变化时按 measured delta 修正 scrollTop。
- 不依赖 end sentinel 每次 `scrollIntoView`。

当用户离开底部阅读历史：

- 新 delta 不强制跳到底部。
- 只更新 unread / scroll-to-bottom affordance。
- history prepend 时保持当前 first visible row 的 visual offset。

## 8. measured row cache

新增：

```ts
type MeasuredRowCache = {
  get(rowId: string): number | undefined
  set(rowId: string, height: number): void
  estimate(row: ThreadRow): number
  invalidate(rowId: string): void
}
```

估算规则：

- user/assistant 短文本：基础高度 + 行数估算。
- code block：按 collapsed/expanded 状态估算。
- tool/process row：按状态类型估算。
- generated files / review card：保守估算。

测量策略：

- `ResizeObserver` 变化进入 batch。
- 每 frame 最多提交一次 measurement update。
- measurement update 后使用 bottom-distance anchor 修正 scrollTop。

## 9. streaming scheduler

当前 main/preload/provider 已有 SSE batch，但 store 仍会随每批 delta 更新 React 状态。

目标新增 renderer scheduler：

```text
SSE deltas -> streaming buffer -> rAF flush -> projection row update -> virtualizer measure
```

策略：

- 每 animation frame 最多 flush 一次 text update。
- 后台窗口或高负载时可降到 30fps 或 15fps。
- tool/status/approval/user-input 这类结构事件仍及时 flush。
- delta 文本只影响 active streaming row，不触发整个 thread rows 重算。

## 10. Markdown/code render 策略

streaming 时：

- 使用轻量 incremental text surface。
- 避免完整 Markdown parse/highlight 每 token 运行。
- code block 可以先 plain text / lightweight shell。

finalized 时：

- 放入 `markdown-finalization-queue`。
- 空闲帧或 turn completed 后再做 rich Markdown / syntax highlight。
- 大代码块可延迟 highlight 或按可见区域 highlight。

这比直接加 Rust 更有效，因为瓶颈主要在 renderer layout/React/Markdown，而不是模型输出文本拼接本身。

## 11. tracing

第一阶段本地 tracing 覆盖：

```text
thread.event.batch_received
thread.delta.buffered
thread.delta.flushed
thread.projection.reduced
thread.rows.updated
thread.virtualizer.measured
thread.scroll.anchor_corrected
thread.react.commit_sample
thread.markdown.finalized
```

记录位置：

```text
Electron userData/traces/thread-*.jsonl
```

不上传远端。

## 12. 迁移顺序

推荐分 6 步：

1. 提取 `deriveTurnSections` / `groupTurns` 到 projection reducer，不改变 UI。
2. 增加 `ThreadRow` 和 row renderer，仍然普通渲染，不启用 virtualizer。
3. 引入 streaming scheduler，把 delta flush 限制到 frame budget。
4. 引入 measured row cache 和 ResizeObserver batch。
5. 替换 `useTimelineScroll` 为 bottom-distance scroll layout。
6. 启用 viewport window virtualizer，保留 load earlier / jump rail / empty hero / fork banner 功能。

## 13. 不做什么

第一阶段不做：

- 不升级 Electron。
- 不把 runtime 改成 Rust。
- 不删除 `MessageBubble` 等现有 UI 组件。
- 不删除旧功能来换流畅度。
- 不强制引入大型第三方虚拟列表库，除非本地实现无法稳定。

Rust 判断：

- 不建议第一阶段用 Rust 重写聊天渲染或 runtime。
- Rust 可后续用于大文件索引、diff、搜索、二进制处理、native perf probe。
- 当前聊天“不丝滑”的最大可见瓶颈在 renderer projection/virtualization/Markdown/scroll anchor。

## 14. 验收标准

功能：

- 长线程可正常加载、滚动、跳转、加载历史。
- streaming 时接近底部自动跟随。
- 用户滚到历史时不被新 token 拉回底部。
- tool/process/review/approval/user-input 顺序正确。
- interrupt / retry / resume / fork / archive 不回退。
- Write / Connect Phone / Schedule 发起的 thread 仍可显示。

性能：

- 100 turns / 500 blocks thread 滚动无明显卡顿。
- streaming active turn 时每秒 React commit 次数受控。
- 长代码块不会在每个 token 重跑完整 highlight。
- history prepend 无明显跳动。
- Windows 上优先验证滚动手感。

测试：

- `useTimelineScroll` 旧测试迁移/保留为 anchor 行为测试。
- 新增 row projection tests。
- 新增 bottom-distance anchor tests。
- 新增 measurement cache tests。
- 新增 streaming scheduler tests。
- 新增 integration test：SSE batch -> projection -> virtual rows。
