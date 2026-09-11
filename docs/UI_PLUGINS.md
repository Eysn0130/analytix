# UI 插件开发指南(形象工坊)

analytix 的「形象工坊」允许任何人制作并安装自己的 mascot 形象包，替换工作台里的泳动主体、欢迎/睡觉/坐着状态、会话 cameo、完成庆祝，甚至主题色和进行中文案。

预装 `mascot` 模式就是官方示例：它不是硬编码功能，而是一个随应用分发、首次启动自动安装的 UI 插件(id 为 `mascot`，见 `src/main/ui-plugin-bundled.ts`)。它在形象工坊里与第三方插件完全同级，可以启用、停用，也可以删除。激活 `mascot` id 时，应用会额外点亮一套手工制作的运球/快攻/喝奶茶动画；第三方插件使用通用泳动/状态动画框架。

一个 UI 插件就是一个文件夹：`manifest.json` + 若干图片。插件是纯声明式的，应用不会执行插件目录里的任何脚本或样式。

```text
my-plugin/
├── manifest.json
└── img/
    ├── swim.png
    ├── greet.png
    └── ...
```

安装方式：`Settings -> 形象工坊 -> 安装插件文件夹...`，选中插件目录即可。应用会校验 manifest 后把 manifest 和被引用到的图片复制进应用数据目录：

```text
~/.analytix/ui-plugins/<id>/
```

源目录中的其他文件不会被复制。

官方示例见 [`examples/ui-plugins/starlight/`](../examples/ui-plugins/starlight/)。

## manifest.json 参考

```json
{
  "id": "starlight",
  "name": "Analytix Cameo",
  "version": "1.0.0",
  "author": "你的名字",
  "description": "一句话介绍(可选,≤240 字符)",
  "figures": {
    "swim": "img/bird.png",
    "surf": "img/surf.png",
    "greet": "img/greet.png",
    "sleep": "img/sleep.png",
    "sit": "img/sit.png",
    "run": "img/run.png",
    "toggleIcon": "img/icon.png"
  },
  "labels": {
    "zh": { "working": "巡航中..." },
    "en": { "working": "Cruising..." }
  },
  "tokens": {
    "light": { "--ds-accent": "#7a5fd0" },
    "dark": { "--ds-accent": "#a78ff0" }
  },
  "features": { "cameos": true }
}
```

## 字段规则

| 字段 | 必填 | 规则 |
| --- | --- | --- |
| `id` | 是 | 2-40 位小写字母/数字/连字符；保留字 `default` / `analytix` / `on` / `off` / `none` 不可用；`mascot` 是预装示例 id，重装会覆盖它 |
| `name` | 是 | 不超过 60 字符 |
| `version` | 是 | 语义化版本，如 `1.0.0` |
| `author` / `description` | 否 | 不超过 80 / 240 字符 |
| `figures` | 是 | 至少一个槽位；路径必须是插件目录内的相对路径，仅 `png/webp/jpg/jpeg/gif` |
| `labels` | 否 | 仅 `zh` / `en` 两种语言；键限 `working` / `workingSprint` / `workingDive` / `workingSurf`；每条不超过 24 字符 |
| `tokens` | 否 | 仅 `light` / `dark` 两个主题；键限 `--ds-*`；值禁止 `url()`、分号、花括号等；总数不超过 60 |
| `features.cameos` | 否 | `true` 时启用主会话两侧的不定时 cameo |

## 形象槽位(figures)

所有图片建议主体朝左、透明背景、最长边 512px 左右。缺失的槽位会回退到默认 analytix mascot 美术，或按下表回退链借用你的其他槽位。

| 槽位 | 出现在哪里 | 缺失时回退 |
| --- | --- | --- |
| `swim` | 回合进行中的泳动动画主体，各处的最终兜底 | 默认 analytix mascot |
| `surf` | 泳动动画的冲浪姿态、庆祝「胜利巡游」 | `swim` |
| `greet` | 欢迎卡片、侧边栏轮播、cameo「探头」、庆祝「跃起欢呼」 | `swim` |
| `sleep` | 运行时唤醒页、侧边栏轮播、cameo「打盹」 | `sit` -> `swim` |
| `sit` | 选择工作区空状态、侧边栏轮播、cameo「歇脚」、庆祝「举杯」 | `greet` -> `swim` |
| `run` | cameo「横穿/对穿」、庆祝「胜利巡游」 | `surf` -> `swim` |
| `toggleIcon` | 形象工坊里的预览小图 | `swim` -> `greet` -> 第一个可用槽位 |

## 体积限制

- `manifest.json` 不超过 64KB。
- 单张图片不超过 2MB。
- 全部图片合计不超过 24MB。

## 安全模型

1. **无代码执行**：插件不含 JS/HTML/CSS 执行入口；即使放了也不会被复制安装。
2. **白名单安装**：只复制 manifest 与被 `figures` 引用的图片；路径禁止 `..`、绝对路径与反斜杠。
3. **图片经主进程读取后以 data URL 注入**，渲染层不直接访问插件目录。
4. **主题 token 白名单**：键名必须是 `--ds-*`，值经过字符集校验，样式文本由应用生成，并锚定在 `html[data-ui-plugin='<id>']` 下，停用即移除。

## 调试技巧

- 安装失败时，设置页会列出 manifest 的具体校验错误。
- 修改插件后重新执行一次「安装插件文件夹...」即可覆盖更新，同 id 覆盖安装。
- 可用的 `--ds-*` token 清单见 `src/renderer/src/styles/base-shell.css` 顶部的 `:root` 与 `[data-theme='dark']` 两个变量块；最常用的是 `--ds-accent` / `--ds-accent-soft` / `--ds-selection`。
