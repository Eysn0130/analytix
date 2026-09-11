# Analytix brand assets / Analytix-derived visual system spec

Status: Normative brand/provenance decisions with historical source inventory.
Current as of: 2026-08-25 for product messaging; 2026-07-10 for the retained
asset lifecycle note.
Source of truth for as-built assets: tracked files, manifests, packaging config,
and visual QA; not the local paths recorded below.

The `/Users/sun/Downloads/analytix-logo` entries preserve original provenance.
They are not portable project dependencies and do not prove that a tracked
asset is current or correctly wired.

## 1. 目的

本 spec 定义 Analytix 第一阶段品牌资产、app icon、tray/dock/installer icon、启动图、空状态、SVG 图标结构、阴影、边框、动效的升级边界。

目标是保留原 Kun 功能和主要信息架构，但把产品身份、图标资产、视觉细节升级为 Analytix，同时参考 CodexDesktop-Rebuild 的成熟视觉结构。

## 2. 冻结决策

- 用户可见品牌统一为 **Analytix**；canonical package、executable、CLI、protocol
  与环境变量前缀继续使用 lowercase `analytix`。营销大小写不改机器级身份。
- 产品类别为 **Agent Platform**；核心品牌句、双语副标题、技术品牌句与架构表达
  由
  [`11-agent-platform-brand-and-architecture.md`](11-agent-platform-brand-and-architecture.md)
  统一定义，本 spec 只负责资产与视觉 provenance。
- `/Users/sun/Downloads/analytix-logo` 只保留为历史 provenance；当前发布资产来源是 tracked `src/asset/brand/` 与 `src/asset/brand/provenance.json`。
- 不保留 Kun app icon。
- 不直接使用 Codex 主 logo。
- CodexDesktop-Rebuild 仅可用于行为、结构、布局和视觉节奏观察；未建立材料级授权前，不复制其代码、二进制提取物、图标、品牌或其他资产。
- 原 Kun 吉祥物/插画/iKun 模式保留功能，第一阶段源码命名改为 `mascot` / `cameo`，后期再替换视觉。

## 3. analytix-logo 历史来源

以下是初始导入时的本机来源记录，不是当前构建依赖：

```text
/Users/sun/Downloads/analytix-logo/analytix-logo-system-readme.md
/Users/sun/Downloads/analytix-logo/analytix-capital-wordmark-logo-x.png
/Users/sun/Downloads/analytix-logo/analytix-capital-wordmark-logo-x-transparent.png
/Users/sun/Downloads/analytix-logo/analytix-exact-complete-logo-system.png
/Users/sun/Downloads/analytix-logo/analytix-exact-symbol-color.png
/Users/sun/Downloads/analytix-logo/analytix-exact-symbol-mono-black.png
/Users/sun/Downloads/analytix-logo/analytix-exact-symbol-mono-white.png
/Users/sun/Downloads/analytix-logo/analytix-exact-symbol-reversed.png
/Users/sun/Downloads/analytix-logo/analytix-exact-app-icon-16.png
/Users/sun/Downloads/analytix-logo/analytix-exact-app-icon-32.png
/Users/sun/Downloads/analytix-logo/analytix-exact-app-icon-64.png
/Users/sun/Downloads/analytix-logo/analytix-exact-app-icon-128.png
/Users/sun/Downloads/analytix-logo/analytix-exact-app-icon-256.png
/Users/sun/Downloads/analytix-logo/analytix-exact-app-icon-512.png
/Users/sun/Downloads/analytix-logo/analytix-exact-app-icon-1024.png
/Users/sun/Downloads/analytix-logo/analytix-image2-logo-lockup-system.png
/Users/sun/Downloads/analytix-logo/analytix-image2-exact-logo-suite-presentation.png
/Users/sun/Downloads/analytix-logo/analytix-image2-brand-application-system.png
```

## 4. 目标项目资产结构

建议：

```text
src/asset/brand/
  analytix-logo-system-readme.md
  analytix-logo.png
  analytix-logo-transparent.png
  analytix-symbol-color.png
  analytix-symbol-mono-black.png
  analytix-symbol-mono-white.png
  analytix-symbol-reversed.png
  analytix-app-icon-16.png
  analytix-app-icon-32.png
  analytix-app-icon-64.png
  analytix-app-icon-128.png
  analytix-app-icon-256.png
  analytix-app-icon-512.png
  analytix-app-icon-1024.png
  analytix-splash.png

build/
  icon.ico
  icon.icns
```

是否保留 `src/asset/img/`：

- 若引用量可控，推荐迁移品牌资产到 `src/asset/brand/`。
- 若短期引用太多，可保留 `src/asset/img/`，但文件名必须是 `analytix*` / `mascot*` / `cameo*`，不能保留 `kun*` / `ikun*` 新身份命名。

## 5. CodexDesktop-Rebuild 参考边界

当前参考仓库位于：

```text
/Users/sun/Projects/_upstreams/CodexDesktop-Rebuild
```

该仓库是下载、提取、补丁和重打包流水线，不是完整未编译的 Codex
Desktop 源码。固定提交没有仓库级项目许可证；README 对原 Codex CLI
Apache-2.0 的说明不能覆盖本仓库脚本、下载的应用、提取代码或资产。
`src/*` 被 Git 忽略，是本机生成的提取输出，既不是 commit-pinned 来源，
也不是授权证明。

使用边界：

- 可参考 icon mask、透明边距、多尺寸输出、tray template 图策略。
- 可参考 empty state 的层级、留白、阴影、动效节奏。
- 可参考 SVG icon 的 stroke、尺寸、hover/active state。
- 不直接替换 Analytix 主 logo。
- 不把 Codex 品牌露出带入 Analytix。
- 不直接复制本机提取目录中的 PNG/WebP/SVG、sprite、minified chunk、
  prompt、二进制或品牌素材。
- 如果另有书面授权，必须先在上游 manifest/来源台账中记录授权主体、
  范围、期限、材料路径与哈希、允许用途和 notice 义务，再修改本 spec。

## 6. 历史迁移前源码冲突点

初始 Kun 资产清单：

```text
src/asset/img/kun.png
src/asset/img/kun_mac.png
src/asset/img/kun_tray.png
src/asset/img/kun_greet.png
src/asset/img/kun_bird.png
src/asset/img/kun_sleep.png
src/asset/img/kun_surf.png
src/asset/img/kun_sit.png
src/asset/img/ikun.png
src/asset/img/ikun_stand.png
src/asset/img/ikun_sleep.png
src/asset/img/ikun_wave.png
src/asset/img/ikun_run.png
src/asset/img/ikun_boba.png
src/asset/img/ikun-ui-plugin.gif
src/asset/img/ikun-ui-plugin.mp4
```

第一阶段处理：

- app icon/tray/dock/installer/icon source 替换为 Analytix 品牌资产。
- `kun_*` mascot 素材改名为 `mascot_*` 或 `cameo_*`。
- `ikun_*` 功能/模式源码改名为 `mascot` / `cameo`。
- 原素材只有在 Kun 许可、required notice 与适用授权范围允许时才可
  保留；文件名和代码语义不能继续表达 Kun 产品身份。当前实际文件以
  tracked `src/asset/img/mascot*` / `cameo*` 为准。

## 7. UI 视觉系统升级方向

不是机械替换所有 SVG，而是按优先级重建视觉规则。

### 7.1 图标

目标：

- 统一 icon size：`14/16/18/20/24`。
- 按按钮密度选择尺寸，不用文字按钮替代常见符号。
- 优先使用项目已有 `lucide-react`，Codex SVG 结构仅用于无对应图标或品牌/状态图。
- 所有 icon button 有 tooltip / aria-label。

需要扫描：

```text
src/renderer/src/components/**
src/renderer/src/write/**
src/renderer/src/sdd/**
```

### 7.2 阴影

当前 Kun UI 中存在大量局部自定义 shadow，例如：

```text
shadow-[0_24px_70px_rgba(...)]
shadow-2xl
shadow-red-950/10
```

目标建立 token：

```text
--ax-shadow-popover
--ax-shadow-modal
--ax-shadow-floating
--ax-shadow-panel
--ax-shadow-focus
```

原则：

- 弹层阴影统一。
- 卡片不嵌套卡片。
- 聊天中间区尽量减少重阴影，降低滚动时 repaint。
- Windows 上避免大面积 backdrop-blur + 多层 shadow 同时出现。

### 7.3 边框

目标 token：

```text
--ax-border-subtle
--ax-border-strong
--ax-border-focus
--ax-border-danger
```

原则：

- 主工作区边框更克制。
- hover/active/focus 状态颜色稳定。
- Chat timeline 不用强边框包每条消息，减少视觉噪声。

### 7.4 动效

Codex-like 体感更像“轻、短、稳定”，不是动画更多。

建议：

- hover transition 控制在 `120ms-160ms`。
- panel open/close 控制在 `160ms-220ms`。
- streaming message 不对每个 token 做 layout animation。
- loading/pending 使用 opacity/transform，不触发昂贵布局属性。
- 尊重 reduced motion。

### 7.5 空状态与启动图

空状态不再使用 Kun 作为品牌露出。

目标：

- Chat empty hero 使用 Analytix logo/symbol。
- Write empty state 使用 Analytix + 当前功能语义。
- Connect Phone 空状态可保留原模式，但用户可见品牌为 Analytix / Connect Phone。
- 启动图使用 Analytix symbol + 简短 loading state。

## 8. 需要保留的 UI/功能

不得删除：

- Chat empty suggestions。
- Workspace picker。
- Work logo / mascot mode。
- Write workspace。
- SDD / plan cards。
- Generated files panel。
- Review plan / summary cards。
- Process/tool sections。
- Connect Phone UI。
- Schedule UI。
- Settings sections。
- UI plugin views。

视觉升级必须保持现有功能入口和操作路径。

## 9. 验收标准

源码检查：

```bash
rg -n "kun.png|kun_mac|kun_tray|ikun|iKun|Kun" src/asset src/renderer src/main src/shared electron-builder.config.cjs
```

期望：

- app icon/tray/dock/installer 不再引用 Kun。
- 用户可见文案不再出现 Kun。
- `mascot` / `cameo` 替代源码中的 `ikun` 模式命名。
- legacy migration 以外不再使用 Kun 作为当前产品身份。

视觉检查：

- macOS app icon 清晰。
- Windows `.ico` 小尺寸清晰。
- tray icon light/dark 可见。
- empty state 不出现 Codex 主 logo。
- empty state 不出现 Kun 主品牌。
- 聊天滚动时阴影/blur 不引起明显 repaint 卡顿。

## 10. 后续可选

- 第二阶段生成新的 mascot/cameo 资产。
- 第二阶段统一 design token 到 `src/renderer/src/styles/tokens.css`。
- 第二阶段建立视觉回归截图。
