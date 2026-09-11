# 本地开发流程

[English](./DEVELOPMENT.md)

> 状态：当前维护者指南。架构边界与文档时效性先查看
> [../AGENTS.md](../AGENTS.md) 和
> [analytix/README.md](./analytix/README.md)。当前源码、测试和 package
> scripts 优先于带日期的历史报告。

## 开发基线

- 本地主线是 `main`。
- 如需短期分支，使用聚焦的前缀，例如 `codex/...`、`feat/...` 或
  `fix/...`。
- 桌面壳是 Electron + React + TypeScript。
- 唯一生产 agent core 是 `packages/runtime-go` 下的 Go runtime。
- `packages/runtime` 负责公开 TypeScript contract/config 和
  `analytix serve` launcher；它不是第二套生产 agent runtime。
- 可用命令以 `package.json` 及其调用脚本为准，不从历史文档或 packaged
  副本推断命令。

项目采用 local-first 维护方式。远端审查或
`.github/workflows/release.yml` 中的验证工作流可以提供补充证据，但不能替代
本地、与改动范围匹配的验证，也不负责生成官方桌面安装包。

## 前置环境

- 与 lockfile 兼容的 Node.js 和 npm；当前验证工作流使用 Node.js 22。
- 修改或验证 `packages/runtime-go` 时，使用 Go 1.22 或更新的兼容工具链。
- 仅在对应范围需要时准备平台工具链：native data tools 使用 Rust，packaged
  analysis backend 使用 Python，官方安装包使用获准的 Windows 或 macOS
  构建环境。

使用 lockfile 精确安装 JavaScript 依赖：

```bash
npm ci
```

## 推荐流程

1. 查看 `git status --short --branch`，保留现有用户改动。
2. 确认当前事实来源、受影响 contract 和可验证的成功条件。
3. 需要时创建短期分支。
4. 实现最小完整改动：覆盖必要 consumer，不夹带无关清理。
5. 运行足以证明改动的最小相关检查。
6. 检查 diff、文档影响和 `git diff --check`。
7. 仅在用户要求或维护流程需要时创建本地 commit。

## 验证

按改动范围选证据，不机械运行所有命令。

| 改动范围 | 预期证据 |
| --- | --- |
| 仅 Markdown | 检查路径/链接并运行 `git diff --check` |
| TypeScript contract 或桌面代码 | `npm run typecheck`、聚焦测试；影响生产产物时再运行 `npm run build` |
| Go runtime | 对改动的 Go 文件运行 `gofmt` 和聚焦 `go test`；涉及 production-only 路径时增加 production tag |
| 跨层 runtime API | 公开 contract、Go adapter/use case、桌面 bridge/consumer 测试、typecheck 和 build |
| UI 行为 | 聚焦测试、`npm run dev` 与人工验证；有助审查时保留截图/视频 |
| 打包或发布 | 在获准主机运行平台 package audit 和相关 release gate |

常用 JavaScript 检查：

```bash
npm run typecheck
npm run test
npm run build
npm run lint
git diff --check
```

`npm run build:runtime` 构建的是 `packages/runtime`，包括 launcher 和公开
TypeScript contracts；它**不会**编译或测试 Go core。

常用 Go 检查：

```bash
(cd packages/runtime-go && go test ./...)
(cd packages/runtime-go && go test -tags analytix_prod ./...)
```

只格式化本次范围内的 Go 文件，例如：

```bash
gofmt -w packages/runtime-go/internal/app/model/example.go
```

RC source freeze 后，只运行一次
`npm run runtime:go:rc-control-plane -- --json` 来验证 source-bound control
plane；即使 product RC 仍被阻塞，它也可能成功，但绝不授权 package 或
release。product release 路径的 runtime 改动使用
`npm run runtime:go:release-gate`，且仅当最终 JSON 的 `passed: true` 时成功
退出。两条命令都不能替代聚焦本地测试。

如检查失败，明确区分本次引入的回归和既有 baseline 问题。未运行或失败的检查
不得报告为通过。

## Runtime 人工检查

启动开发应用：

```bash
npm run dev
```

启动诊断应结合日志、实际进程、监听端口 owner 和公开 health endpoint：

```bash
curl http://127.0.0.1:<runtime-port>/health
```

`GET /v1/runtime/info` 默认需要 runtime bearer token，只有显式 insecure
启动时例外。问题记录中不得粘贴该 token。公开 CLI 只支持
`analytix serve`；版本证据应来自 Settings/About 和 runtime diagnostics，
不要假设还存在其他 CLI subcommand。

## 本地发布

开发打包命令包括：

```bash
npm run dist
npm run dist:mac
npm run dist:win
npm run dist:linux
```

维护者发布入口：

```bash
npm run release:mac
npm run release:win
```

- 官方 Windows 安装包必须在获准的 Windows host 构建。
- `npm run release:win` 调用 `scripts/release-win.ps1`，生成 Standard
  Windows bootstrapper、运行 package audit，并采用该脚本实现的 cache、
  archive、channel、签名和可选 R2 配置。
- Authenticode 默认作为可选证据；只有设置
  `ANALYTIX_REQUIRE_WINDOWS_AUTHENTICODE=1` 才成为硬要求。native binary、
  data-engine single-owner gate 和 forbidden-file 检查始终是硬要求。
- 生成的 `dist*/`、`output/`、package extraction、报告和本地 release
  metadata 是证据或产物，不是实现源码。

机器特定 workspace、cache、archive、certificate 和发布凭证应保存在获准的
本地配置中，不写入维护文档。

## 变更说明

有效的变更说明应记录：

- 改了什么以及为什么；
- 用户可见或兼容性影响；
- 实际执行的命令与人工检查；
- 已知 baseline 失败或尚未具备的外部条件；
- 交互改动在适合时附截图或视频。

使用聚焦的 Angular-style commit message，例如：

- `fix(runtime): preserve SSE replay cursor`
- `feat(settings): add provider endpoint format`
- `docs(development): correct Go validation path`
