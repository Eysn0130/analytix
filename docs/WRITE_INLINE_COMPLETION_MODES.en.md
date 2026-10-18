# Shared Markdown inline completion

Status: Reference. Scope: shared Markdown editors used by Plan and SDD.
The `write:*` names are implementation identifiers, not a standalone product entry.
For the accepted NewDoc → Word direction, see [product scope](analytix/product-completion.md#delivery-scope).
This source map does not establish fresh runtime, Provider, native Word or installed acceptance.

## Current owners

- [PlanPanel](../src/renderer/src/components/plan/PlanPanel.tsx) and
  [SddDraftEditorView](../src/renderer/src/components/sdd/SddDraftEditorView.tsx)
  mount the shared Tiptap editor with a CodeMirror fallback and completion settings.
- [WriteRichEditor](../src/renderer/src/write/tiptap/WriteRichEditor.tsx) and
  [WriteMarkdownEditor](../src/renderer/src/components/write/WriteMarkdownEditor.tsx)
  retain their respective suggestion and accept/dismiss interactions.
- [Document request binding](../src/renderer/src/write/inline-completion/request.ts)
  supplies thread, request and document identity for shared editor requests.
- [Main completion service](../src/main/services/write-inline-completion-service.ts)
  uses the [Runtime inline-completion contract](../packages/runtime/src/contracts/inline-completion.ts),
  through main-owned transport, rather than a direct DeepSeek FIM endpoint.

## Behavior and boundaries

Short and long completion policies share context construction and scoring. The
response protocol can also represent an edit of an available bounded candidate;
it does not authorize a rewrite outside that candidate. Settings and current
request ownership still control whether a request may run or its result apply.
Renderer/IPC checks and Runtime document bindings remain separate responsibilities.

[Retrieval](WRITE_RETRIEVAL_RAG.en.md) and
[recent edits](WRITE_INLINE_EDIT_RECENT_EDITS.en.md) provide bounded context.
For explicit SDD selection replacement, see [inline editing](WRITE_INLINE_EDIT_RAG.en.md).
The earlier standalone Write/FIM workflow is not current product guidance.
