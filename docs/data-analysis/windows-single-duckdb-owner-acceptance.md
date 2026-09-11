# Windows Single DuckDB Owner Acceptance

> 类型：当前验收流程，不是历史通过证明。目标 Windows package、目标 commit
> 和目标环境必须重新运行本文件中的适用检查并保存受控 evidence。

## 当前安全状态

Windows 下 active `case.duckdb` 最终必须只有一个 Go 宿主管理的
`analytix-data-engine.exe` owner。Python backend、Electron main、
`analytix-analysis-compute.exe` 和 `analytix-cleaning-ops.exe` 都不得绕过该
权威直接打开 active DB。

P0 期间，Electron `DataAnalysisBackendManager` 已固定为终止性的
`data_analysis_native_authority_unavailable`：它不解析 Python/backend 路径，
不创建目录，不探测端口，不启动 Uvicorn，不执行 HTTP fallback，也不通过环境变量
选择解释器。打包的 Python/backend 文件即使仍作为迁移资产存在，也不是生产可达
能力。Renderer 收到终止 blocker 后不得轮询或回退到默认 loopback endpoint。

因此当前 Windows 数据分析路径不得宣称 single-owner ready。只有以下 Go 权威切片
实现并在 Windows 实机通过后，才可恢复 stats/chart/flow/cleaning：

1. target-specific native registry 校验完整 SHA-256、签名、架构和 receipt；
2. Windows suspended start 与 no-breakaway、kill-on-close Job Object；
3. bounded request/response、deadline、cancel、输出预算和确认式进程树终止；
4. Renderer -> preload -> main -> Go proxy -> data engine 的严格代理；
5. 同案 context、dataset snapshot、grant 与 evidence gate；
6. package E2E 证明所有旧 Python/helper 直连路径不可达。

## P0 打包闸门

在批准的 Windows 主机上构建：

```powershell
npm run dist:win
node scripts/audit-windows-package.cjs
```

当前 package audit 必须机械证明：

- Electron data-analysis manager 保留固定 native-authority blocker；
- 生产 import graph 不含 Electron/Python/Uvicorn 启动器；
- `ANALYTIX_DATA_ANALYSIS_PYTHON`、backend/path/port selector 不可达；
- 资金 MCP 生产入口不导入 Python、DuckDB workbench 或本地 tool runtime；
- 打包 Python/backend 资产只能是 dormant migration asset，不能被报告为已受 Go 管理；
- package 中没有运行后生成的 `__pycache__` 或 `*.pyc`；
- ordinary runtime info、SSE、日志和 UI 不暴露 PID、端口、可执行路径、DB 路径、
  raw error、完整账号或案件标识。

P0 实机检查：

1. 启动 packaged Analytix 并打开案件分析页。
2. 预期获得固定的 native-authority unavailable 边界，不是空数据、零值或成功状态。
3. 保持页面打开至少两个旧轮询周期，确认没有每 15 秒重试。
4. 确认没有 Python/Uvicorn/data-analysis backend 进程、监听端口、工作目录或 job 记录。
5. 修改 `ANALYTIX_DATA_ANALYSIS_PYTHON`、backend/path/port 环境变量后重复启动，
   确认 marker 程序没有执行且结果不变。
6. MCP 断线、旧 catalog 或 dormant packaged backend 存在时，仍不得产生案件事实、
   evidence receipt 或正式报告。

任一直接启动、fallback、轮询、文件/job 副作用或事实发布都属于 P0 失败。

## Go 权威恢复后的 single-owner 验收

只有恢复前置条件全部实现后，才运行：

```powershell
node scripts/verify-data-engine-single-owner.cjs --runtime-dir <packaged resources/runtime> --db-path <test case.duckdb> --case-id <case_id>
cargo test --manifest-path tools/data_engine/Cargo.toml windows_named_mutex_conflict_returns_owner_conflict
```

`verify-data-engine-single-owner.cjs` 当前属于迁移诊断，不能单独证明生产调用图、
Job Object、Go proxy、case binding 或 evidence gate。非 Windows 的 `skipped` 结果也不
计为通过。

恢复后的 Windows package E2E 必须同时证明：

1. stats、chart、flow 和 cleaning 全部经 Go proxy 到同一个 data engine owner；
2. active `case.duckdb` 只由注册且签名匹配的 `analytix-data-engine.exe` 持有；
3. 第二 owner 得到固定 `data_engine_owner_conflict`，不得变成 `INTERNAL_ERROR`；
4. cancel、timeout、崩溃、重启和 app quit 后不存在 detached/breakaway descendant；
5. stale case/epoch/grant 的迟到结果被拒绝且不进入 UI、history、report 或 export；
6. raw rows 和完整账号只进入同案、授权、审计且 evidence-bound 的受控 artifact；
7. 普通诊断只含固定 code、布尔值、计数和不透明引用。

## 受控发布证据

PID、可执行路径、DB 路径、owner file、原始 DuckDB message 和完整账户字段不得进入
普通日志或普通诊断导出。确需排障时，只能由发布工程师在受控 evidence artifact 中
采集，并记录目标 package hash、commit、主机、授权、保留期和访问审计。面向普通
UI/SSE/日志的证据只保留：

- fixed blocker/code；
- source readiness boolean；
- owner/termination/result counts；
- opaque component、snapshot 和 execution references；
- receipt/claim coverage，不含原始案件字段。

## 失败与通过判定

以下任一项失败即阻断发布：Electron/Python 直启或 fallback、环境 selector 生效、
旧 helper 直连 active DB、未确认进程树终止、ordinary diagnostics 泄露本地路径/PID/
raw error/PII、无同案证据发布事实、非 Windows `skipped` 被计为 Windows pass。

通过必须同时满足 P0 零副作用隔离、Windows Go authority/Job Object 实机验证、
single-owner 并发验证、桌面全链路、package audit 和 Final Evidence Gate。仅有二进制
文件、mutex 单测、健康 ping 或可打开 DB 均不构成通过证明。
