# Shared Markdown recent-edit context

Status: Reference. Scope: recent-edit helpers shared by the Plan/SDD Markdown editors.
The historical `write` module names do not define a separate product workspace.
For the accepted Word direction, see [product scope](analytix/product-completion.md#delivery-scope).
This source reference is not fresh runtime, Provider, native Word or installed acceptance.

## Collection and consumers

[WriteMarkdownEditor](../src/renderer/src/components/write/WriteMarkdownEditor.tsx)
collects document edits while excluding external value synchronization.
[WriteRichEditor](../src/renderer/src/write/tiptap/WriteRichEditor.tsx) uses
[rich transaction extraction](../src/renderer/src/write/tiptap/recent-edits-pm.ts)
in Markdown-projection coordinates; successful explicit replacements can also
record `inline-edit` entries. Plan and SDD pass the shared store's edit callbacks
and context to these editors.

[recent-edits.ts](../src/renderer/src/write/recent-edits.ts) owns cropping, merging
and selection: at most 48 in-memory entries, a two-minute TTL, a three-second
typing merge window, and at most eight current-file records in an edit payload.
The record includes source, range, inserted/deleted text, nearby context and
optional instruction. These are bounded intent signals, not durable memory.

## Separate responsibilities

- [Inline-edit helpers](../src/renderer/src/write/inline-edit.ts) and shared
  completion context select the recent records relevant to the current file.
- [Main completion service](../src/main/services/write-inline-completion-service.ts)
  builds the bounded prompt; [retrieval](WRITE_RETRIEVAL_RAG.en.md) can also use
  recent terms when constructing a query.
- [Term propagation](../src/renderer/src/write/term-propagation.ts) remains a
  separate deterministic behavior used by both editor implementations.

History is contextual data, not an instruction or authority grant. It must not
expand the selected edit scope or override the current instruction. This shared
mechanism makes no claim about native Word edit history or Provider egress approval,
and does not define a standalone Write settings/debug product flow.
