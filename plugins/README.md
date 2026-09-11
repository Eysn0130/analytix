# Analytix Plugin Development Layout

本目录是 Analytix 自研 Agent 插件的主开发入口。Analytix 的产品架构是
**Go Agent Harness + Plugins + Privacy Layer**：唯一 Go Harness 拥有执行与隐私
安全底线，插件通过类型化 capability 扩展专业工具、工作流、Skills 和安全 UI，不得
创建第二 runtime 或绕过 Provider、持久化、日志、公共事件和本机敏感显示边界。
后续新增或维护插件时，优先把源码放在这里，目录名必须与
`.codex-plugin/plugin.json` 里的 `name` 保持一致。

面向协作说明读本文件；面向后续 Agent 的强约束见 `AGENTS.md`。

## 推荐目录

| 目录 | 用途 | 是否主开发源 |
| --- | --- | --- |
| `plugins/<plugin-name>/` | analytix 自研插件源码；包含 manifest、skills、assets、MCP server、脚本和插件级文档 | 是 |
| `plugins/<plugin-name>/hub/` | Hub marketplace / skills / install-policy 的本地生成结果或 fixture；仅插件需要通过 Hub 发布时使用 | 否，发布材料 |
| `vendor/<package-name>/` | 被桌面 app 打包进 runtime 的本地 npm 依赖或二进制包镜像 | 否，依赖镜像 |
| `dist/` | Electron / packaging 生成产物 | 否，禁止开发 |
| `~/.codex/plugins/cache/` | Codex 本机安装缓存 | 否，禁止开发 |
| `analytix-hub/agent-plugin-sources/analytix-hub/...` | Hub 仓库内的发布源镜像，用于线上 marketplace 检测和发布 | 否，发布镜像 |

## 当前插件

| 插件 | 主开发目录 | 发布/打包关系 |
| --- | --- | --- |
| `analytix-fund-analysis` | `plugins/analytix-fund-analysis/` | Analytix 首个旗舰专业插件；用于证明平台可进入资金分析与其他高敏感专业业务。桌面安装包通过 `electron-builder.config.cjs` 复制到 `resources/plugins/analytix-fund-analysis`；Hub 发布时再同步到 analytix-hub 源。 |
| `analytix-computer-use` | `plugins/analytix-computer-use/` | 插件展示、skills 和图标在本目录；真正 runtime 后端包来自 `vendor/analytix-computer-use` 这个 npm file dependency。 |
| `atlasflow` | `plugins/atlasflow/` | 本仓库存放 analytix 定制插件源和 Hub catalog fixture；目前不随桌面安装包 extraResources 直接内置。 |

## 新插件放置规则

1. 新插件统一放在 `plugins/<plugin-name>/`，不要放到 `vendor/`、`dist/`、`~/.codex/plugins/cache/` 或 Hub 镜像目录里直接开发。
2. `<plugin-name>` 使用小写 hyphen-case，且必须等于 `.codex-plugin/plugin.json` 的 `name`。
3. 最小结构：

   ```text
   plugins/<plugin-name>/
   ├── .codex-plugin/plugin.json
   ├── README.md
   ├── assets/
   └── skills/
   ```

4. 如果插件暴露 MCP server，增加 `.mcp.json` 和 `mcp/`。
5. 如果插件需要 Hub 发布，增加 `hub/`，并用插件自己的 `scripts/prepare-hub-package.mjs` 或 Hub 发布脚本生成 marketplace / skills / install-policy。
6. 如果插件依赖独立二进制或 npm 包，把可打包依赖放在 `vendor/<package-name>/`，但插件 manifest、skills 和用户可见资产仍保留在 `plugins/<plugin-name>/`。

Funds 不定义 Analytix Core。未来 Knowledge、Legal、Research、Writing、Coding
等专业能力必须复用同一个插件 identity、capability、lifecycle、privacy 和故障
隔离模型；任何领域语义都不应固化进通用 Go Harness。

## 同步边界

- `plugins/` 是开发事实来源；修改插件能力、manifest、skills、图标和本地 MCP 逻辑时从这里开始。
- `vendor/` 只保存桌面 app 构建需要的依赖镜像。独立项目更新后，再把发布包同步进 `vendor/`。
- `analytix-hub/agent-plugin-sources/...` 是 Hub 发布镜像。只有通过发布流程同步，不把它当日常开发入口。
- 发布前必须比较 `plugins/<plugin-name>/` 与 Hub 发布镜像；发现差异时，以 `plugins/<plugin-name>/` 为事实源运行对应同步/发布脚本。
- `dist/` 和安装缓存是可删除产物。发现同名插件时，以 `plugins/` 和对应独立源仓库为准。

## 验证建议

- 修改 manifest 后先跑 JSON 校验或插件 validator。
- 修改 MCP 或跨层调用后跑相关插件脚本和最小 runtime 测试。
- 修改打包关系后跑 `npm run build`，必要时再跑对应 `npm run dist:*` 或 packaging audit。
