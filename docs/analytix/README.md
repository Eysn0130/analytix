# Analytix 文档地图

Status: Operational。此页只路由当前责任与证据；不保存实时 writer、候选或验收状态。

## 按当前问题读取

| 当前问题 | 唯一入口 | 继续读取的条件 |
| --- | --- | --- |
| 应怎样工作 | 根目录与受影响目录的 `AGENTS.md` | 仅沿目标文件的目录链读取 |
| 应实现什么 | [spec registry](specs/README.md)、`openspec/specs/<capability>/spec.md` | 仅相关 accepted scope；active change 必须由当前请求授权 |
| 当前实现和缺口 | 当前代码、测试、Git diff；[产品矩阵](product-completion.md) | 产品交付需要对应行时读取，不把矩阵或旧 PASS 当成 fresh evidence |
| 长期交付次序 | [执行方案](delivery-execution-plan.md) | 任务涉及 Core/Funds 或交付顺序时读取 |
| 暂停与恢复 | [交接入口](handovers/README.md) | fresh 核对 Git、index、保护状态与有效 writer |
| 本机构建与执行 | [开发基线](development-baseline.md)、[runbook](development-runbook.md#development-routes) | 使用相关命令时读取；同 shell 加载缓存 helper |
| Git 与外部整合 | [Git workflow](git-workflow.md) | 仅在当前授权的提交/同步/整合范围内 |
| 上游引入与许可证 | [upstream admission](upstreams/README.md) | 复制或改编第三方输入前按精确版本核验 |
| 有界知识镜像 | [knowledge base](knowledge-base.md) | authoritative source 更新后按直接相关范围同步 |

普通维护不递归打开这些链接。日期 QA、旧 handover、archive、legacy、
生成物与 packaged 副本仍可按具体事实问题查询；它们只证明记录的候选与环境。
历史入口见 [QA index](qa/README.md)、[归并登记](document-consolidation-register.md)
与 [legacy index](../legacy/README.md)，不会自动成为下一施工步骤。

## 当前架构与 authority

桌面边界为 Renderer → `window.analytix` → preload → main → Go HTTP/SSE。
唯一生产 Agent core 是 `packages/runtime-go/internal/runtimeapp`；
`packages/runtime` 提供公共 TypeScript contracts/config/telemetry 与 Go launcher。
`ANALYTIX_RUNTIME_BACKEND=typescript` 仅用于 retired-backend 诊断。

产品表达为 Go Agent Harness + Plugins + Privacy Layer。Funds 为旗舰专业插件；
Privacy Layer 内建于同一个 Harness，模型、普通持久化与 protected-local 展示
遵循不同投影与权限合同。普通启动使用本地 Provider Registry / Secret Store；
Hub 仅在显式兼容操作时延迟加载。运行时 settings 顶层为 `runtime`，Provider 为
`provider`。详细目标以 [registry](specs/README.md) 与相关 accepted capability 为准。

分别报告 `as-built`（代码与当前结果）、`target`（接受范围）与 `gap`。
Task checkbox、artifact `done`、archive 日期都不是当前实现、安装或发布 PASS。
workspace-write 是应用层 policy；不得称为 OS 沙箱。不同候选、平台、合成/真实
Provider、源码/安装/发布之间不转移验收结论。

## 派生状态与按改动验证

复用 [validation-burden](../../scripts/validation-burden.mjs) 与现有 CI/test 矩阵：

```bash
source ./scripts/use-analytix-cache.sh
node scripts/validation-burden.mjs --json
node scripts/validation-burden.mjs --plan <task-owned-paths> --json
openspec status --change <selected-change> --json
```

默认只输出当前入口和 immediate active changes 的派生 task counts，不读取
archive 或日期 QA 正文。`--inventory` 保留原全仓静态盘点；静态数量不选掉测试。
已映射的有界维护 fixture 返回 focused plan；未知范围返回 `unmapped`，须按受影响
owner 与现有矩阵选验证，不据此跳过隐私、权限、架构、平台、tag 或 CI 门禁。
TS producer fixture 的路线包含现有 typecheck，以检查实际消费者；仅修改该测试
文件时可使用窄测试路线。fixture 路线不证明其他源码的依赖覆盖。

`--measure docs-only|local-typescript|go-authority` 对现有固定 fixture 各执行三轮，
记录驱动实际读取字节/哈希/跳转、必要命令与墙钟成本。它是维护路线演练，无产品
修改；子验证器内部读取未计量，也没有修复前后延迟基线，不能推导模型 token、
费用、一般生产效率或分析能力。运行时仍需上述缓存 helper。
