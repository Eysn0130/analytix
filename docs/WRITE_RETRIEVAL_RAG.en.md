# Shared Markdown BM25 and keyword retrieval

Status: Reference. Scope: retrieval supporting the shared Plan/SDD Markdown path.
This is not a standalone Write workspace or native Word operation guide.
For the accepted NewDoc → Word direction, see [product scope](analytix/product-completion.md#delivery-scope).
This source map does not establish fresh runtime, Provider, native or installed acceptance.

## Authority and owners

[Main IPC](../src/main/ipc/register-app-ipc-handlers.ts) captures the current
renderer owner and obtains a thread-bound source through
[write-retrieval-ipc](../src/main/ipc/write-retrieval-ipc.ts).
Core authorizes, validates and scans the workspace. The renderer's workspace
path is only an assertion to match against that authority, not permission to
choose an arbitrary directory for main-process scanning.

[write-retrieval-service](../src/main/services/write-retrieval-service.ts) builds
its bounded in-memory index from the authorized source snapshot. Cache/source
currentness is not defined by a workspace-root string alone. The service checks
source validity and request ownership before returning snippets.

## Retrieval behavior

The service chunks text, tokenizes English and Chinese, ranks with BM25 plus
keyword/title/path signals, and returns bounded reference snippets. Inline
completion excludes the current file and PDFs; the separate retrieval path can
include PDFs and controls current-file inclusion. It is not a persistent knowledge
index or permission to expose every file in a workspace.

The [completion service](../src/main/services/write-inline-completion-service.ts)
requests up to three snippets for short completion or five for long/edit and
eligible edit candidates. Retrieval can be disabled; missing, empty or failed
optional retrieval can yield no context. Generation still requires its own
current Runtime request/document authority and does not revert to the old direct
DeepSeek FIM route.

See [completion](WRITE_INLINE_COMPLETION_MODES.en.md),
[selection editing](WRITE_INLINE_EDIT_RAG.en.md) and
[recent edits](WRITE_INLINE_EDIT_RECENT_EDITS.en.md) for shared consumers.
Retrieved content is reference data, not instructions or an authority grant.
