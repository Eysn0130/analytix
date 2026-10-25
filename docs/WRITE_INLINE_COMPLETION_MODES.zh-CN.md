# 共享 Markdown 行内补全

状态：Reference。范围：Plan 与 SDD 使用的共享 Markdown 编辑器。
`write:*` 名称是实现标识，不代表独立产品入口。
已接受的 NewDoc → Word 方向见[产品范围](analytix/product-completion.md#delivery-scope)。
本页仅映射源码，不证明 fresh runtime、Provider、原生 Word 或安装验收通过。

## 当前 owner

- [PlanPanel](../src/renderer/src/components/plan/PlanPanel.tsx) 与
  [SddDraftEditorView](../src/renderer/src/components/sdd/SddDraftEditorView.tsx)
  挂载共享 Tiptap 编辑器、CodeMirror fallback 及补全设置。
- [WriteRichEditor](../src/renderer/src/write/tiptap/WriteRichEditor.tsx) 与
  [WriteMarkdownEditor](../src/renderer/src/components/write/WriteMarkdownEditor.tsx)
  保留各自的建议展示、接受与取消交互。
- [文档请求绑定](../src/renderer/src/write/inline-completion/request.ts)
  为共享编辑器请求提供 thread、request 与 document 身份。
- [主进程补全服务](../src/main/services/write-inline-completion-service.ts)
  经 main transport 使用 [Runtime inline-completion 合同](../packages/runtime/src/contracts/inline-completion.ts)，
  不再采用直接请求 DeepSeek FIM endpoint 的旧路线。

## 行为与边界

短补全和长补全策略共享上下文构造与评分。响应协议也能表达对已有有界候选的
编辑，不授予候选范围外的重写权限。设置和当前请求 owner 仍决定请求能否执行、
结果能否应用；renderer/IPC 检查与 Runtime 文档绑定各有独立责任。

[检索](WRITE_RETRIEVAL_RAG.zh-CN.md)与
[最近编辑](WRITE_INLINE_EDIT_RECENT_EDITS.zh-CN.md)提供有界上下文。
SDD 显式选区替换见[行内编辑](WRITE_INLINE_EDIT_RAG.zh-CN.md)。
旧独立 Write/FIM 工作流不再作为当前产品指引。
