# 共享 Markdown BM25 与关键词检索

状态：Reference。范围：服务于共享 Plan/SDD Markdown 路线的检索能力。
本页不是独立 Write 工作区或原生 Word 操作指南。
已接受的 NewDoc → Word 方向见[产品范围](analytix/product-completion.md#delivery-scope)。
本页仅映射源码，不证明 fresh runtime、Provider、原生或安装验收通过。

## Authority 与 owner

[Main IPC](../src/main/ipc/register-app-ipc-handlers.ts)捕获当前 renderer owner，
通过 [write-retrieval-ipc](../src/main/ipc/write-retrieval-ipc.ts)取得 thread 绑定的
source。Core 负责 workspace 的 authorize、validate 与 scan。
renderer 提供的 workspace 路径仅用于和已授权范围匹配，不授予 main 任意扫描
目录的权限。

[write-retrieval-service](../src/main/services/write-retrieval-service.ts)基于已授权
source snapshot 构造有界内存索引。缓存/source 的 currentness 不由 workspace-root
字符串单独决定；返回片段前仍检查 source 有效性与请求 owner。

## 检索行为

服务对文本分块、中英文分词，使用 BM25 加关键词/标题/路径信号排序并返回有界
参考片段。行内补全排除当前文件与 PDF；独立检索路线可纳入 PDF，并另行控制
是否包含当前文件。它不是持久知识索引，也不授予暴露 workspace 全部文件的权限。

[补全服务](../src/main/services/write-inline-completion-service.ts)为短补全最多请求
三个片段，为 long/edit 及具备条件的编辑候选最多请求五个。检索可关闭；缺少、
为空或失败的可选检索可不提供上下文。生成仍须满足自身 Runtime 请求/文档的
当前 authority，不会回退到旧的直接 DeepSeek FIM 路线。

共享消费者说明见[补全](WRITE_INLINE_COMPLETION_MODES.zh-CN.md)、
[选区编辑](WRITE_INLINE_EDIT_RAG.zh-CN.md)及
[最近编辑](WRITE_INLINE_EDIT_RECENT_EDITS.zh-CN.md)。
检索内容是参考数据，不是指令或 authority。
