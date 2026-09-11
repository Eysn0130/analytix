# Analytix Plugin Agent Guide

本指南补充 `plugins/` 的目录级规则，并继承根 `AGENTS.md`。本目录是 Analytix
自研插件的开发事实源；插件清单以当前目录和各 manifest 为准，不在本文件维护。

## Source And Publication Boundaries

- 插件源码默认位于 `plugins/<plugin-name>/`。采用 Analytix package contract
  的包以 `.analytix-plugin/package.json` 的 `packageId`、`packageVersion`
  为身份与版本来源；`.codex-plugin/plugin.json`、marketplace 和 MCP metadata
  是派生或经可执行一致性检查的消费投影，不能成为独立版本或 capability owner。
  未迁移的包先核对其实际打包合同，不通过文档编辑补造 manifest 或 grant。
- `plugins/<plugin-name>/hub/` 是 Hub 发布材料或 fixture；`vendor/` 是桌面打包
  依赖；`dist/` 是构建产物；`~/.codex/plugins/cache/` 是安装缓存。这些位置不是
  日常插件开发入口。
- Hub 或其他发布镜像由主插件源生成或同步。比较差异时，以主源和发布流程为准，
  不用镜像反向覆盖开发源。
- 插件依赖独立后端 package 时，在该 package 的事实源完成修改和验证，再通过
  既有同步或发布流程更新消费侧材料。

## Change And Validation

- 先从任务涉及的 manifest、入口或文档定位相关 skill、asset、MCP server 或
  脚本，按实际调用与打包关系扩展读取；不因修改一个文件扫描全部插件内容。
- 修改 manifest 后，验证其所属 schema、canonical identity/version 与消费投影
  一致，并确认 `skills`、`mcpServers` 等引用存在。解析成功或文件存在不能替代
  Host admission、签名 binding 与当前 capability grant。
- 文档或目录规则变更运行 `git diff --check`；MCP/runtime 变更运行插件自己的最小
  相关检查；影响桌面消费或打包时，再增加对应 build 或 packaging evidence。
- 外部 skill、代码、prompt 或 asset 的借鉴遵循根指南的固定 commit、许可证、
  provenance 和 admission 规则。
