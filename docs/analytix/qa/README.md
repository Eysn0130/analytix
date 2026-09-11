# QA 文档边界

Status: Operational index
Applies to: `docs/analytix/qa/`
Source of truth: current code/specs plus a fresh rerun on the stated platform

这个目录同时包含操作手册、时点报告和截图，三者不能互相替代。

## Operational runbook

- [`windows-qa-operator-runbook.md`](windows-qa-operator-runbook.md) 定义当前
  Windows QA 主机的无密连接方法、SSH alias、RDP/RustDesk fallback 和
  Analytix Hub managed gateway live-provider smoke 方法及 incident gate。它
  不是某次 QA 已通过的证据。

## Dated evidence

其余带日期的 Markdown 和截图是历史证据快照，只对报告中记录的 commit、
worktree、平台、环境、命令和结果状态有效。

引用这些证据时必须区分：

- `pass`：对应命令和范围确实运行并通过；
- `partial`：只覆盖声明的部分；
- `skipped`：有意未运行；
- `blocked`：存在未满足的外部或产品 gate；
- `not_configured`：环境未配置，不能解释成 pass。

文件名含 `final`、旧报告写着 `passed`、截图看起来正常或文件仍被 Git 跟踪，
都不能证明当前 worktree、当前 Windows package 或当前 release channel 已通过。
要做当前时态结论，必须重跑当前命令；无法重跑时应引用原 commit 和环境并明确
称为 historical evidence。

## Secret and privacy rules

- 允许在 operational runbook 中记录批准的 LAN 地址、hostname、account、端口
  和不含值的 secret-manager locator。
- 禁止记录密码、private key、product key、remote-control identifier、永久
  access code、API key、token、恢复码或能够还原这些值的命令输出。
- QA screenshot、terminal transcript、JSON/JSONL、package log 和 runtime output
  也受同一规则约束；生成目录不是秘密存储位置。
- Windows runbook 或引用路径改变后运行：

  ```bash
  npm run verify:windows-qa-runbook
  ```

  checker 只报告 path、line 和 rule name，不回显匹配值。
- history rewrite 后运行 `npm run verify:windows-qa-runbook:history`，确认
  本地 retained refs 无法再访问 retired path，且 retained text object 的
  Windows-access credential scan 通过；这不替代 credential rotation。

历史清理不能撤销仍有效的 credential，credential rotation 也不能删除已复制的
Git blob。2026-07-10 已完成本 checkout 的 rotation、retained-ref purge、
redacted rescan、reflog/unreachable-object 清理及临时恢复 bundle 删除，且当前
没有 Git remote，因此本地 incident closure 已完成。其他 owner 独立保留的重写
前 clone、bundle、backup 或 release archive 不在本地闭环边界内；未经同等清理
与验证不得向本仓库重新导入 refs 或 objects。
