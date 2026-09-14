# Protected local text editing

Status: Reference. Describes the text editor implementation; it does not establish
Office-format support, AI proposal admission, GUI acceptance or release readiness.

The desktop editor opens an existing text file through
`window.analytix.objects.request` and the typed
`/v1/local-display/object-editing` lane. Main restricts this IPC to its own main
frame. Core binds the object session to the current installation principal,
workspace and canonical file identity. Cursor movement, typing and formatting
remain local. Browser-only previews cannot use this private lane.

Each save freezes a content revision and operation ID. Core compares the actual
file bytes, preserves its supported text encoding and mode, and uses the existing
atomic replacement primitive. Receipt metadata is persisted before replacement
and contains hashes and operation state, without source text or raw paths. Reads
and replacements have a 1.5 MiB bound on encoded and decoded text; receipts have
a 16 KiB bound. Symlinks, metadata roots and protected host paths are rejected.

A lost response retains the exact operation and draft. Retry first queries the
receipt; a restarted runtime requires a new session for the same object. A
historical committed receipt is checked against current disk bytes before the
editor displays Saved. Input entered while a save is running remains dirty.
Conflict comparison displays the retained draft and disk version; saving after
review uses that exact disk revision, and another change requires a new review.
Closing a document, folding the panel and cancelling an agent remain distinct.

Current limits: editing sessions and drafts are in memory, so this is not crash
recovery of unsaved drafts. An indeterminate operation whose original bytes
remain on disk stays unresolved rather than being silently rewritten. The new
atomic editing capability is available on macOS/Linux. On an explicitly reported
unsupported platform the existing manual editor remains labelled Compatibility
save; storage, identity and permission failures never activate that fallback.
Existing agent write/edit review still describes an already-persisted edit and
does not become a pre-commit proposal through this text-save API.

Focused verification is in `internal/app/objectediting`, the object editing
HTTP/composition/snapshot and filestore tests, and the renderer object-editing
and Main IPC tests. Source the repository cache helper before running them.
