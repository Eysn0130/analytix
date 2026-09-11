# Legacy 文档边界

Status: Historical index
Applies to: `docs/legacy/`
Source of truth: 无；当前行为回到代码、accepted specs 和
`docs/analytix/README.md`

本目录保存 former Kun、旧架构或迁移来源材料，目的是支持 provenance、历史
比较和迁移审计。它不是当前 Analytix 的规范、操作手册、产品身份或实现证据。

当前子目录：

- `kun/`：former Kun 架构、缓存、hooks 和 contributing 的中英文历史副本。

引用这些文件时必须写明 `historical`，并核对当前 Analytix 边界：

- 生产 Agent Core 只有 `packages/runtime-go`；
- 桌面 bridge 是 `window.analytix`；
- 产品、CLI、环境变量和发布身份是 `analytix`；
- TypeScript Agent Runtime 已退役；
- 旧 Kun/Reasonix 路径或命名不能作为当前实现建议直接复制。

不要在本目录新增当前 runbook、验收报告或 active design。新历史材料必须说明
来源、日期、许可证/授权和为何仍需保留。
