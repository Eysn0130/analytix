# 共享 Markdown 最近编辑上下文

状态：Reference。范围：Plan/SDD Markdown 编辑器共享的 recent-edit helper。
历史 `write` 模块名不代表独立产品工作区。
已接受的 Word 方向见[产品范围](analytix/product-completion.md#delivery-scope)。
本页是源码说明，不证明 fresh runtime、Provider、原生 Word 或安装验收通过。

## 采集与消费者

[WriteMarkdownEditor](../src/renderer/src/components/write/WriteMarkdownEditor.tsx)
采集文档编辑，并排除外部 value 同步。
[WriteRichEditor](../src/renderer/src/write/tiptap/WriteRichEditor.tsx)通过
[富文本 transaction 提取](../src/renderer/src/write/tiptap/recent-edits-pm.ts)
使用 Markdown 投影坐标；成功的显式替换也可记录 `inline-edit`。
Plan 与 SDD 将共享 store 的编辑回调和上下文传给这些编辑器。

[recent-edits.ts](../src/renderer/src/write/recent-edits.ts)负责裁剪、合并和筛选：
内存中最多 48 条，TTL 两分钟，连续打字合并窗口三秒，编辑 payload 最多携带
八条当前文件记录。记录包含来源、范围、删除/插入文本、邻域及可选指令。
它们是有界意图信号，不是持久记忆。

## 独立责任

- [Inline-edit helper](../src/renderer/src/write/inline-edit.ts)与共享补全上下文
  筛选当前文件相关记录。
- [主进程补全服务](../src/main/services/write-inline-completion-service.ts)构造有界
  prompt；[检索](WRITE_RETRIEVAL_RAG.zh-CN.md)也可用最近术语构造查询。
- [术语传播](../src/renderer/src/write/term-propagation.ts)仍是两个编辑器共同使用的
  独立确定性行为。

历史是上下文数据，不是指令或 authority；不得扩大选区范围或覆盖当前指令。
这一共享机制不证明原生 Word 编辑历史或 Provider 出站批准，
也不定义独立 Write 的设置/调试产品流程。
