# analytix TypeScript Runtime 边界包

Status: Operational package reference。

`packages/runtime` 不是生产 agent 实现。它负责：

- 公共 `analytix serve` TypeScript launcher；
- 共享 HTTP/SSE contracts 与 Zod schemas；
- launcher/config 解析和 telemetry helpers；
- Go runtime 演进期间使用的 TypeScript conformance/regression oracles。

唯一生产 agent core 位于 `packages/runtime-go`。`analytix serve` 启动 Go
`runtime-server`，同时保持公共 ready marker 和 CLI 边界稳定。
`ANALYTIX_RUNTIME_BACKEND=typescript` 已退役，不能启动 TypeScript agent
loop。

生产组合与 Go 内部结构见 [runtime-go/README.md](../runtime-go/README.md)。
desktop settings、launcher parsing、同步配置与 Go 实际消费字段之间的区别见
[ANALYTIX_CONFIG.md](../../docs/ANALYTIX_CONFIG.md)。

## 生产 surface

`tsconfig.build.json` 只输出公共边界：

```text
src/
  cli/          analytix serve 参数解析与 Go runtime launcher
  config/       共享配置 schema 与脱敏 helpers
  contracts/    公共 HTTP/SSE schemas 与 types
  telemetry/    共享 usage/cache telemetry helpers
  hooks/        为配置兼容保留的 schema types
```

`loop-test-support`、`server-test-support`、`model-test-support`、
`tool-test-support`、`services-test-support` 与 `conformance` 都是测试/oracle
来源。已退役的 TypeScript loop、server、model client、tool host、delegation
与 review 实现属于迁移参考，不能继续承载新的生产 agent 行为。

这条规则不能泛化到 `tsconfig.build.json` 之外的所有文件。Electron main 当前
仍直接引用少量 source helper，包括 `adapters/file/atomic-write.ts` 和
`adapters/computer-use/backend-factory.ts`。它们虽然不进入发布 package build，
但 desktop 使用是真实的。给其他 adapter/service 定性前必须先检查 import graph。

## Scripts

在 `packages/runtime` 下执行：

```bash
npm run typecheck
npm run test
npm run build
npm run serve -- --data-dir ~/.analytix/data
```

`npm run build` 只构建 TypeScript launcher/contracts 包，不编译或验证
`packages/runtime-go`。Go 改动需要运行 focused Go tests：

```bash
(cd ../runtime-go && go test ./...)
```

生产敏感改动可能还需要：

```bash
(cd ../runtime-go && go test -tags analytix_prod ./...)
```

## CLI 边界

当前唯一公共 command 是：

```text
analytix serve [options]
```

当前 dispatcher 没有公共 `run`、`chat`、`exec` 或 `--version` command。

有效 launcher options 包括：

| 参数 | 含义 | 默认值 |
| --- | --- | --- |
| `--config <path>` | launcher 支持的 serve/provider JSON 配置 | 存在时读取 `<data-dir>/config.json` |
| `--host <host>` | runtime bind host | `127.0.0.1` |
| `--port <port>` | runtime port | `8899` |
| `--data-dir <path>` | runtime durable data root | `serve` 必填 |
| `--runtime-token <token>` | `/v1/*` Bearer token | empty；local insecure 语义取决于启动模式 |
| `--api-key <key>` | standalone provider API key | empty |
| `--base-url <url>` | standalone 默认 provider base URL | `https://api.deepseek.com/beta`；GUI/managed provider 会覆盖 |
| `--model-proxy-url <url>` | 上游模型 proxy | empty |
| `--endpoint-format <format>` | `chat_completions`、`responses`、`messages` 或 `custom_endpoint` | `chat_completions` |
| `--model <id>` | 默认 model id | `deepseek-v4-pro` |
| `--approval-policy <policy>` | 默认 tool approval policy | `on-request` |
| `--sandbox-mode <mode>` | 默认应用层 tool/path policy | `workspace-write` |
| `--insecure` | 显式本地开发时关闭 Bearer auth | off |

示例：

```bash
analytix serve \
  --host 127.0.0.1 \
  --port 8899 \
  --data-dir ~/.analytix/data \
  --runtime-token dev-token \
  --model deepseek-v4-pro
```

桌面端通常由 Electron main 同步 GUI-managed config 后直接启动 Go binary。
packaged app 携带平台原生 `runtime-server`，不依赖用户安装 Go toolchain 或随包
Go source。

launcher 与新建 Go thread 默认使用 `approvalPolicy: on-request` 加
`sandboxMode: workspace-write`。workspace-write 是应用层 tool/path policy，
不是操作系统沙箱，并会阻止前台/后台主机 shell；只有显式选择
`danger-full-access` 才允许主机 shell 与不受限文件访问。桌面 normalization
只把未版本化且恰好是旧默认 `auto` 加 `danger-full-access` 的组合迁移一次；其他
有效组合和 version-2 full-access opt-in 保持有效。无人值守 Connect Phone 与
scheduled-task turn 使用 `never` 加 `workspace-write`。

## 配置边界

不能从“schema 接受该字段”推断生产行为已经生效。

- launcher 从 JSON config 解析 serve/provider 值和 `modelProviders`，并把适用
  字段传给 Go。
- Electron main 同步 `<dataDir>/config.json`，再把路径传给 Go core，供当前已
  接线的 capability settings 使用。
- Go production composition root 是
  `packages/runtime-go/internal/runtimeapp/app.go`。只有字段从 config/launch
  input 进入该 composition，并继续进入 Go use case/adapter，才能称为
  production-active。
- 一些 TypeScript-era schema keys 为兼容仍可接受或写入，但当前 Go production
  path 未消费，包括 command-hook/quality execution、SQLite/hybrid storage
  selection、model-based context-compaction settings、token-economy behavior，
  以及可配置 tool-storm/argument-repair behavior。它们是 implementation gap，
  不是当前已启用功能。

普通 packaged desktop 使用本地 Provider 引导及 dataDir-scoped Registry / Secret
Store authority。Provider 凭据不能写入 desktop settings、`config.json`、示例或
日志。Hub 仅保留显式延迟加载的兼容入口，不是默认 Provider 或凭据 fallback。
详见[配置指南](../../docs/ANALYTIX_CONFIG.md)。

## Runtime data 与 API owner

Go core 管理 `<dataDir>` 下的 durable runtime data，包括 thread JSONL、events、
attachments、memory、task jobs 和 runtime-owned worktrees。旧 TypeScript
SQLite/index layout 不能继续作为当前生产说明。

当前 routes 与实现入口：

- `packages/runtime-go/internal/runtimeapp/app.go`
- `packages/runtime-go/internal/adapters/inbound/httpapi/`
- `packages/runtime-go/internal/adapters/inbound/sse/`
- `packages/runtime-go/internal/adapters/outbound/eventlog/`
- `packages/runtime-go/internal/adapters/outbound/filestore/`

renderer/main 必须继续使用本 package 的公共 contracts。Go implementation
detail 或 conformance route 不能变成新的 renderer-visible product protocol。

## 故障排查

- launcher 找不到 Go runtime：使用受支持的 runtime environment 指向有效的
  packaged binary，或在包含 `packages/runtime-go/go.mod` 的 checkout 中使用
  compatible Go toolchain。
- runtime 不可达：同时检查真实 child process、exit status、listener owner、
  `/health` 与 `/v1/runtime/info`；不要将未经脱敏的 child stderr 作为公开证据。
- Provider 404：核对 selected provider、model、Base URL、endpoint format 与
  最终脱敏 request URL。
- 配置看似被忽略：先确认字段只是被 TypeScript schema 接受/写入，还是确实被
  `runtimeapp` 与对应 Go service 加载。
