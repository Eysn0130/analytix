# 标签工作区与原生文档标注

Status: Reference / 当前候选验收记录。2026-09-14。
分支 `codex/workbench-product-delivery-20260914`，起始 HEAD
`1d7a500ad401e3a5d9e0936e02c36df4aff7758c`；以下源码仍属本轮工作区候选，
不以起始提交冒充新候选 SHA，也不构成可发布或正式 Office 分发准入。

## 范围

用户在执行过程中明确取消新建手动 Office 编辑器的目标，改为原生标注、
主对话沟通与 AI 受控修改。具体研究与取舍见
[标注交互研究](../upstreams/native-document-annotation-2026-09-14.md)。
原有 Markdown/文本编辑能力保留；不新增手动 Office 字体、公式、段落、复杂撤销重做。

## 当前候选

| 面向用户的行为 | 当前实现与验证边界 |
| --- | --- |
| 全局布局 | 两个按钮控制右侧工作区和底部终端；功能选择器与对象/工具 Tabs 位于工作区。 |
| 标签生命周期 | 手动键盘激活、关闭最后一项返回选择器、折叠恢复活动项；主会话不因工具切换重建。 |
| 文件浏览 | Files Tab 与 Documents 停靠文件栏复用同一树；Office、文本与代码走对应对象打开路径。 |
| 原生标注 | 文本/单元格/对象选择、可选备注、主输入框引用；不自动发送，不覆盖已写输入。 |
| 无文本对象/复杂范围 | 附带位置的讨论引用；不能宣称已经取得精确可写范围。 |
| AI 修改 | 同一主线程工具读取 Core scope 并生成提案；UI 接受后 Main 校验对象、版本、序列和插件 generation，执行精确文本替换。 |
| 应用修改 | 红删绿增提案；接受后自动执行有界 OOXML 导出、摘要/CAS/Core 提交；仅回执确认后显示已更新。 |
| 撤销 | 修改成功后才显示，恢复前一份文件；绑定最后提交版本，外部冲突不覆盖，未知回执查原操作，重载时重新检查当前文件。 |
| 文件操作 | “打开”下拉：默认应用、在文件夹中显示、内部浏览；原生文档重复标题栏及常驻保存/导出按钮移除。 |
| 原生交互 | ReadOnly/LockEditDoc 恒为 true；用户导航、选择、复制，正文键入/粘贴/格式命令拒绝。 |
| 宏与分发 | 明确 NEVER_EXECUTE=0；source-experiment 与 packaged 拒绝、插件 publishable:false 保留。 |

## 已取得证据

- `npx vitest run --pool=threads` 的十五个原生/工作区/打开操作测试文件：137/137 通过。
  覆盖精确重复文本目标、宏0、手动操作拒绝、陈旧目标、自动保存、单次撤销、
  回执重试、冲突和隐藏后的迟到打开。其后新增明确失败、重载后外部变更、
  未确认更新退出保护，相关三个测试文件再次运行：44/44 通过。
- IPC、公开对象契约、文件树、summary 和 workspace helpers 九个关联测试文件：76/76 通过。
- NativeOfficePanel 与 native-reference-store：22/22 通过。
  覆盖备注、不会调用 setInput/发送、失焦保留、跨对象/线程/版本失效、
  无文本图形讨论以及新 scope 下旧引用降为讨论快照。
- 两个既有 Write/主线程回归：48/48 通过，使用真实 objects.request 契约 fixture，
  包含 open/commit/版本/摘要回执、重开与现有主对话引用。
- 顶层 domain conformance 指定回归通过；使用 TypeScript AST 排除嵌套方法，
  精确补齐 HEAD 已有的 office/packageHost/objects 三域。
- `npm run typecheck` 通过（web 与 node）。
- 最终收敛候选的 runtime TypeScript、Main、preload、Renderer 完整构建通过，
  固定源清单中的 6173 个文件与 canonical 源码核对，非文档源码漂移为零。
  `npm run typecheck` 再次通过；改动文件 ESLint 0 errors，保留一个既有 Hook 依赖 warning。
  构建通过不等于 GUI 或 Provider 验收。

原生固定 WASM 独立合成验证：XLSX、PPTX 已验证恒只读模型下原生选区、
受控替换、导出和重开；DOCX 同一路径发现导出自身修改通知导致序列误增，
以同步导出范围内抑制通知修复后通过，并补充异常后恢复跟踪的回归。
各格式证据记录自己的 worker 哈希；PPTX/XLSX 的较早成功不能冒充最后哈希的全量验收。
这些引擎实验没有经过应用 Core 持久化服务，不能代替应用保存旅程。

## 待闭合与不得扩大声明

- 当前应用 GUI 的标注→主对话→Diff→应用并自动保存→撤销/重开连续旅程：blocked。
  既有合成 QA 身份的 Keychain 已锁定，原控制台入口不可恢复；已按正常方式退出
  未进入工作区的测试应用，未新建身份、复制凭据、绕过解锁或触碰真实 Provider。
  该环境阻塞不等于产品旅程通过，也不把所有启动错误归因为 Keychain。
- 当前候选 PR/完整 mandatory CI 与对应 SHA。
- 旧全量测试在范围修正前启动，暴露基线 fixture 漂移及若干本机封包相关失败，
  又发生 worker 启动超时；已停止该过期运行，其结果不作为本候选 gate PASS。
  已修复与本工作区/对象契约直接相关的 fixture；未弱化正式封包或签名检查。
- mock、真实 Provider、引擎实验、应用 GUI 和 packaged 分发须分别记录。
