# Shared Markdown selection editing and retrieval

Status: Reference. Scope: the shared Markdown edit helpers and SDD call site.
This is not guidance for a standalone Write workspace or the native Office surface.
The accepted NewDoc → Word direction remains in [product scope](analytix/product-completion.md#delivery-scope).
Source call sites do not establish fresh runtime, Provider, native or installed acceptance.

## Current path

[SddDraftEditorView](../src/renderer/src/components/sdd/SddDraftEditorView.tsx)
constructs explicit inline-edit requests from a single selection and instruction.
The shared [inline-edit helpers](../src/renderer/src/write/inline-edit.ts) resolve
that selection and produce `prefix`, `suffix`, `original`, an edit candidate and
filtered [recent edits](WRITE_INLINE_EDIT_RECENT_EDITS.en.md).

The resolved scope is the exact selected range. The old description of automatically
expanding a short selection into its natural paragraph is no longer accurate.
[Same-paragraph term propagation](../src/renderer/src/write/term-propagation.ts)
is a separate deterministic editor behavior, not expansion of the AI edit scope.

## Applying a result

The SDD call site checks the selected document and original text before applying
source-text replacement. In rich mode,
[WriteRichEditor](../src/renderer/src/write/tiptap/WriteRichEditor.tsx) maps Markdown
projection offsets, verifies the original text and applies a Markdown transaction.
A successful replacement can record an `inline-edit` recent-edit entry.
Saving belongs to the host editor/store lifecycle, not a retired Write autosave route.

The [main completion service](../src/main/services/write-inline-completion-service.ts)
can add [authorized retrieval context](WRITE_RETRIEVAL_RAG.en.md) and parses bounded
completion/edit actions through the Runtime route. All required IPC thread,
request, document and owner bindings must still be satisfied; describing these
helpers is not proof that every caller completes that end-to-end contract.
Retrieved text and edit history remain reference data, not permission to edit
other ranges or bypass current authority.
