# analytix release / packaging / channel spec

Status: Normative release identity plus historical first-stage audit.
Current as of: 2026-08-26 for the implementation addendum below.
Source of truth for as-built packaging: `electron-builder.config.cjs`,
`electron-builder.standard-win.cjs`, package scripts, and packaging tests.

Current implementation addendum: general user-visible product, desktop,
shortcut and uninstall copy uses `Analytix`; canonical package/executable/app
id/CLI/env/data-directory identity remains `analytix`. The official Standard
Windows installer has a deliberate localized display exception:
product/shortcut/menu/uninstall copy is `Analytix灵鉴`, and its artifact pattern is
`analytix-standard-${version}-${arch}.${ext}`. The executable remains
`analytix`, and app id remains `com.analytix.desktop`. The configured default
feed is `https://analytix.top/desktop/releases/standard-win/`; availability,
metadata correctness, signing, and platform QA still need release-time proof.

## 1. 目的

本 spec 定义 analytix 第一阶段发布、打包、更新、artifact、env、license/author、本地项目元数据、平台产物范围。

第一阶段目标是建立新的 analytix 发布身份，不兼容旧 deepseek-gui / KunAgent/Kun 更新路径。

## 2. 冻结决策

| 项目 | 结论 |
| --- | --- |
| productName | canonical/default `Analytix`; official Standard Windows display exception `Analytix灵鉴` |
| package name | `analytix` |
| app display name | canonical/default `Analytix`; official Standard Windows `Analytix灵鉴` |
| shortcut / installer / uninstall name | canonical/default `Analytix`; official Standard Windows `Analytix灵鉴` |
| executable name / userData directory | `analytix` |
| appId / bundle id | `com.analytix.desktop` |
| Windows AppUserModelID | `com.analytix.desktop` |
| artifactName | general `analytix-${version}-${os}-${arch}.${ext}`; official Standard Windows `analytix-standard-${version}-${arch}.${ext}` |
| release prefix | `analytix` |
| update channels | `stable` / `beta` |
| default channel | `stable` |
| env prefix | `ANALYTIX_*` |
| license | `Apache-2.0`；随包第三方义务由 `THIRD_PARTY_NOTICES.md` 独立承载 |
| author | `Guoqin He (GitHub: Eysn0130)` |
| repository | 不配置；analytix 是本地项目，不绑定 GitHub 仓库 |

## 3. release URL

The channel-shaped URL below is the original portable target and remains an
optional `ANALYTIX_RELEASE_BASE_URL` composition mode. It is not the current
default feed. Current configs default to
`https://analytix.top/desktop/releases/standard-win/` unless an explicit feed
or release base URL overrides it.

目标：

```text
https://<release-domain>/analytix/channels/<channel>/latest/
```

建议 env：

```text
ANALYTIX_RELEASE_BASE_URL=https://<release-domain>/analytix
ANALYTIX_UPDATE_CHANNEL=stable
ANALYTIX_APP_VERSION=0.1.0
ANALYTIX_DIST_DIR=dist
```

拼接规则：

```text
${ANALYTIX_RELEASE_BASE_URL}/channels/${ANALYTIX_UPDATE_CHANNEL}/latest/
```

禁止：

- 不使用 `https://www.kun-agent.com/api/r2` fallback。
- 不使用 `deepseek-gui` release prefix。
- 不读取 `KUN_*`。
- 不读取 `DEEPSEEK_GUI_*`。
- 不支持 `frontier` channel。

## 4. 平台产物范围

第一阶段：

```text
Windows: NSIS x64
macOS:   dmg + zip, arm64 + x64
Linux:   AppImage x64
```

后续：

```text
Windows: arm64 / MSIX
Linux:   deb / rpm
```

第一阶段 artifact 示例：

```text
analytix-0.1.0-win-x64.exe
analytix-0.1.0-mac-arm64.dmg
analytix-0.1.0-mac-arm64.zip
analytix-0.1.0-mac-x64.dmg
analytix-0.1.0-mac-x64.zip
analytix-0.1.0-linux-x64.AppImage
```

实际 `${os}` 值以 `electron-builder` 输出为准，但文件名前缀必须 lowercase `analytix-`。

## 5. 当前源码冲突点

Currentness note (2026-06-20): 本节是第一阶段迁移开始前的初始审计冲突
清单，不是当前源码事实。当前执行依据以
`/Users/sun/Projects/analytix/AGENTS.md`、spec `06` sections `22` / `23`、
spec `08` section `24`、以及最新 upstream / QA ledgers 为准。当前源码
已使用 `analytix` canonical identity、`ANALYTIX_*` release env、Go runtime
binary packaging、`stable` / `beta` channels，并配置 `analytix.top` feed。
仍需在 release time 证明 feed 可用性/metadata、local provenance、签名/公证、
Windows NSIS 实机验证和 packaged QA；“尚无真实 URL”不再是当前事实。

`package.json` 当前冲突：

- `name` 是 `kun-gui`。
- `productName` 是 `Kun`。
- `description` 含 `Kun runtime`。
- `author` 是 `Kun Contributors`。
- `homepage` / `repository` 指向 `KunAgent/Kun`。当前 analytix 本地项目不应配置 GitHub repository metadata。
- scripts 使用 `build:kun`。
- scripts 清理 `dist/Kun-*` / `dist/DeepSeek-GUI-*`。

`electron-builder.config.cjs` 当前冲突：

- env helper 为 `envWithLegacyFallback('KUN_*', 'DEEPSEEK_GUI_*')`。
- 默认 URL 是 `https://www.kun-agent.com/api/r2`。
- 默认 prefix 是 `deepseek-gui`。
- channel 允许 `stable` / `frontier`。
- `appId` 是 `com.xingyuzhong.deepseekgui`。
- `productName` 是 `Kun`。
- `artifactName` 是 `Kun-${artifactVersion}-${os}-${arch}.${ext}`。
- `asarUnpack` / `files` 指向 `kun/`。
- mac microphone permission 文案含 `Kun uses...`。
- mac/Linux icon 指向 `kun*` 图片。
- NSIS shortcut/uninstall 名称是 `Kun`。

## 6. electron-builder 目标结构

建议配置语义：

```js
const releaseBaseUrl = (process.env.ANALYTIX_RELEASE_BASE_URL || 'https://<release-domain>/analytix')
  .trim()
  .replace(/\/+$/, '')

const updateChannel = normalizeUpdateChannel(process.env.ANALYTIX_UPDATE_CHANNEL || 'stable')
const genericUpdateUrl = `${releaseBaseUrl}/channels/${updateChannel}/latest/`
const releaseAppVersion = (process.env.ANALYTIX_APP_VERSION || '').trim()
const artifactVersion = releaseAppVersion || '${version}'
```

channel validation：

```js
function normalizeUpdateChannel(raw) {
  const value = String(raw || '').trim()
  if (value === 'stable' || value === 'beta') return value
  throw new Error(`ANALYTIX_UPDATE_CHANNEL must be "stable" or "beta", got: ${raw}`)
}
```

核心 config：

```js
module.exports = {
  appId: 'com.analytix.desktop',
  productName: 'Analytix',
  executableName: 'analytix',
  artifactName: `analytix-${artifactVersion}-\${os}-\${arch}.\${ext}`,
  publish: [{ provider: 'generic', url: genericUpdateUrl }],
  directories: {
    output: process.env.ANALYTIX_DIST_DIR || 'dist'
  }
}
```

## 7. asar / runtime packaging

随着 `kun/` -> `packages/runtime/`，打包规则同步改为：

```text
packages/runtime/dist/**/*
packages/runtime/package.json
packages/runtime/package-lock.json
packages/runtime/node_modules/**/*
```

`asarUnpack` 同步改为：

```text
**/packages/runtime/dist/**/*
**/packages/runtime/package*.json
**/packages/runtime/node_modules/**/*
```

保留 native deps：

```text
better-sqlite3
node-pty
bindings
file-uri-to-path
```

## 8. 签名与 notarization

第一阶段保留当前签名流程的能力，但命名改为 analytix：

```text
MAC_SIGN
APPLE_API_KEY_ID
APPLE_API_ISSUER
APPLE_API_KEY
APPLE_API_KEY_BASE64
CSC_LINK
CSC_NAME
CSC_KEY_PASSWORD
```

这些不是品牌 env，可暂时保留通用命名。

仍需确认：

- Windows code signing certificate。
- macOS Developer ID / notarization 账号。
- 是否要单独的 beta 签名/发布 bucket。

v1.0.1 Windows 标准安装包发布例外：

- 经产品确认，`安装包未签名：SmartScreen 风险仍在。` 仅作为已知风险记录，不作为 v1.0.1 Windows 封包、发布和验收阻塞项；后续正式代码签名证书仍按本节待办继续跟进。

## 9. update channel 策略

`stable`：

- 默认 channel。
- 面向普通用户。
- app 内更新默认检查 stable。

`beta`：

- 内部测试和早期用户。
- 用户必须显式切换或安装 beta 包。
- beta 不自动升 stable，除非后续定义跨 channel 策略。

禁止：

- 不继续使用 `frontier`。
- 不在同一安装包里默认混用 stable/beta。

## 10. 验收标准

源码检查：

```bash
rg -n "Kun-|DeepSeek-GUI|deepseek-gui|kun-agent|KUN_UPDATE_CHANNEL|DEEPSEEK_GUI|frontier|Kun Contributors|KunAgent/Kun" package.json electron-builder.config.cjs scripts
```

期望：

- 除迁移说明/历史文档外无命中。

构建检查：

```bash
npm run typecheck
npm run build
npm run dist:win
npm run dist:mac
npm run dist:linux
```

第一阶段至少验证：

- `npm run typecheck`
- `npm run build`
- packaging config unit tests

## 11. 风险

- 旧 appId 变更意味着这是新 app identity，不是旧 Kun app 的原地升级。
- 用户旧数据不会自动复用，除非设计 one-shot migration/import。
- release domain 未定前只能用 placeholder，不能发布真实 auto-update。
- 代码签名未定前，Windows/macOS 正式分发会有系统信任提示。
