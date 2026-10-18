# 共享 Markdown 选区编辑与检索

状态：Reference。范围：共享 Markdown 编辑 helper 与 SDD 调用点。
本页不描述独立 Write 工作区或原生 Office surface 的操作路线。
已接受的 NewDoc → Word 方向仍见[产品范围](analytix/product-completion.md#delivery-scope)。
源码调用点不证明 fresh runtime、Provider、原生或安装验收通过。

## 当前路径

[SddDraftEditorView](../src/renderer/src/components/sdd/SddDraftEditorView.tsx)
从单个选区和指令构造显式 inline-edit 请求。
共享 [inline-edit helper](../src/renderer/src/write/inline-edit.ts)解析选区，构造
`prefix`、`suffix`、`original`、编辑候选及已筛选的
[最近编辑](WRITE_INLINE_EDIT_RECENT_EDITS.zh-CN.md)。

当前 scope 是精确选区；旧文“短选区自动扩展到自然段”的描述已不准确。
[同段术语传播](../src/renderer/src/write/term-propagation.ts)是独立的确定性编辑器
行为，不代表 AI 编辑范围自动扩大。

## 应用结果

SDD 调用点在源码文本替换前检查当前文档与原文。富文本模式由
[WriteRichEditor](../src/renderer/src/write/tiptap/WriteRichEditor.tsx)映射 Markdown
投影坐标、核验原文并应用 Markdown transaction。
替换成功后可记录一条 `inline-edit` recent edit。
保存属于宿主编辑器/store 生命周期，不沿用已退休独立 Write 的 autosave 叙述。

[主进程补全服务](../src/main/services/write-inline-completion-service.ts)可加入
[经授权检索上下文](WRITE_RETRIEVAL_RAG.zh-CN.md)，通过 Runtime 路线解析有界
补全/编辑 action。IPC 所需 thread、request、document 和 owner 绑定仍须全部满足；
描述这些 helper，不等于证明每个调用方已完成端到端合同。
检索文本与编辑历史是参考数据，不授予其他范围的编辑权限，也不能绕过当前 authority。
