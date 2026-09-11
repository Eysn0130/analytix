# 本地维护说明

[English](./CONTRIBUTING.md)

analytix 采用 local-first 维护方式。本指南定义一组改动应具备的范围、审查、
验证和记录；命令与环境准备参见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

## 从当前事实开始

- 修改前阅读从根目录到改动所属目录路径上的所有适用 `AGENTS.md`；规则冲突时
  以范围更具体的指南为准。
- 使用 [analytix/README.md](./analytix/README.md) 区分 active spec、历史证据、
  生成产物和 legacy 材料。
- 对带日期报告中的现在时结论，先用当前源码、测试和 package scripts 核实。
- 本地主线是 `main`。新分支默认使用 `codex/...`，除非维护者要求其他前缀。
- 从 `git status --short --branch` 开始，保留无关的用户改动。

## 形成最小完整改动

好的改动是最小的**完整**改动，而不只是文件数量最少。它应当：

- 解决一个明确问题或结果；
- 覆盖该结果所需的所有 contract consumer；
- 提供聚焦测试或其他合适证据；
- 行为或流程变化时更新 active 文档；
- 排除推测性功能、alias、大范围重构和格式化噪声。

runtime API 改动应按实际影响走完整路径：公开 schema、Go use case 与 HTTP/SSE
adapter、Electron main/preload、renderer consumer 及聚焦 contract tests。不要只修
可见 UI，也不要把生产行为接回已退役的 TypeScript runtime。

## 验证要求

按改动范围选择证据：

- 仅 Markdown：检查路径/链接并运行 `git diff --check`。
- TypeScript：运行 `npm run typecheck` 和聚焦测试；可能影响生产产物时增加
  `npm run build`。
- Go runtime：对改动文件运行 `gofmt`，并在 `packages/runtime-go` 下运行
  聚焦 `go test`；Go 1.22 是语言基线。
- production-only Go 路径：适用时增加
  `(cd packages/runtime-go && go test -tags analytix_prod ./...)`。
- UI：运行相关测试，并通过 `npm run dev` 人工检查流程；截图或视频能实质帮助
  审查时予以保留。
- packaging/release：在获准平台运行相关 package audit 和 release gate。

TypeScript 命令 `npm run build:runtime` 不会验证 Go core。RC source freeze 后，
只运行一次 `npm run runtime:go:rc-control-plane -- --json` 来验证 source-bound
control plane；它不授权 package 或 release。只有改动到达 product release
路径时才使用 `npm run runtime:go:release-gate`，且仅当最终 JSON 的
`passed: true` 时成功退出。

如实列出实际运行的检查。如检查失败，区分本次引入的回归与既有 baseline 失败。

## 审查标准

审查应关注：

- 正确性和回归风险；
- 是否符合 accepted specs 与当前架构；
- IPC、HTTP/SSE、filesystem、provider 和 migration input 的边界校验；
- 安全与脱敏；
- 适用时的 UX 清晰度和可访问性；
- 测试/证据是否相关，而不是单纯追求测试数量；
- 文档时效性与迁移影响；
- 每项有意改动是否服务于当前目标，以及不可避免的生成差异是否经过检查。

## 文档

更新距离改动最近的 active 文档：

- `README.md` / `README.en.md`：产品级 setup 和 usage；
- `docs/DEVELOPMENT.md` / `docs/DEVELOPMENT.zh-CN.md`：本地工作流；
- `docs/ANALYTIX_CONFIG.md`：配置边界；
- `docs/analytix/specs/`：accepted target behavior，仅在目标决策本身改变时更新；
- 当前指南：维护与审查标准。

不要刷新带日期的 QA/release report 后就静默宣称它是当前事实。应记录对应 commit、
环境、命令和结果，否则继续把它视为历史快照。

## 安全

不得提交或粘贴：

- API key、account token、runtime bearer token、password、private key 或
  product key；
- remote-control ID 或在线 host credential；
- 未脱敏 provider response、environment dump 或机器私有路径。

使用获准的 secret storage 和脱敏 placeholder。如果 live secret 曾被提交，仅从
当前文件删除并不充分；必须轮换 secret，并协调有意图的 history/remotes/backups
清理。

## 问题记录

应包含：

- 操作系统及版本；
- Settings/About 中显示的 analytix app 版本；
- 被测试的 commit 或 package identity；
- 精确复现步骤以及预期/实际行为；
- 已脱敏日志、错误文本和截图；
- startup 问题涉及的实际 desktop/runtime process path 和 listener owner；
- `GET /health` 结果，以及仅在已授权时提供脱敏后的
  `GET /v1/runtime/info` 摘要。

公开 runtime CLI 只支持 `analytix serve`，没有通用 version subcommand。
版本证据使用 Settings/About 和已授权的 runtime diagnostics。不得包含
authenticated runtime-info 请求使用的 bearer token。

## Commit 与协作

使用聚焦的 Angular-style message，例如：

- `feat(scope): short description`
- `fix(scope): short description`
- `docs(scope): short description`

变更说明应写清改了什么、为什么、用户可见或 migration 影响，以及实际完成的
验证。即使使用 remote issue、pull request 或 CI workflow 作为补充协作界面，
本地审查与验证仍然需要完成。

以尊重、清晰、建设性的方式协作；遇到 destructive、protocol-changing、
migration-changing 或 externally visible 决策时，先对齐方向。

## 许可证

项目基于 [Apache License 2.0](../LICENSE) 发布。提交贡献也表示你同意仓库中的
[Contributor License Agreement](../CLA.md)。

Analytix 的第一方版权人及 Project Owner 是 Guoqin He，其公开 GitHub
账号为 [Eysn0130](https://github.com/Eysn0130)。贡献者仍保留 CLA 中对其自身
Contribution 明确规定的权利。
