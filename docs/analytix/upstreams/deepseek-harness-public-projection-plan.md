# DeepSeek Harness public presentation projection plan

Status: Reference / proposed stage 2 design. No public protocol or production Go
implementation is changed by the renderer stage 1 candidate.
Base: Analytix `09003e3f185bd0255cb227a9f96f1cc03b683fdd`.
Research pin: DeepSeek Harness `5badb15009ae1756c3afe0ae0cef1faafc290ccc`.

## Current authority and gap

`src/shared/public-runtime-content.ts` rejects `assistant_text_delta`.
`public-runtime-sse.ts` has a closed event vocabulary and receipt-specific
publication paths. `ToolResultPublicProjectionV1` in
`packages/runtime/src/contracts/items.ts` currently admits only withheld,
host status, plan status, artifact status, case source status and MCP diagnostic
metadata. It sets private payload withheld, fact answering disallowed and no
evidence authority. The renderer port preserves those boundaries.

Consequently stage 1 can display public lifecycle/status, existing final answer
projections, questions and approvals. Rich read/diff/search/terminal/web result
cards and ordinary incremental assistant text remain dependent on a separately
accepted Go publication design. Synthetic display fixtures prove renderer
behavior only; they do not establish a public producer.

## Proposed closed payloads

Each new payload would carry a version, explicit kind, thread/turn/item identity,
publisher-issued projection identity and generation, durable sequence and safe
status. No arbitrary object, generic JSON, raw tool arguments, model reasoning,
or unclassified error body would be an allowed field. Unknown kind/version and
identity mismatches fail closed before IPC, persistence and rendering.

| Kind | Proposed display fields | Producer proof and boundary |
| --- | --- | --- |
| `assistant_display_segment_v1` | bounded public text, segment index, append/replace operation, final flag, display-only classification | Go issues only after the existing output authority approves publication; private model deltas cannot be relabeled as segments |
| `shell_display_v1` | approved command label, process state, bounded redacted public output, truncation flag, exit status | execution policy and foreground/background authority retained; no interactive PTY channel or arbitrary environment/arguments |
| `read_display_v1` | validated workspace-relative display path, line range, bounded public excerpt, truncation, revision | existing filesystem/case permission authority; paths and snippets must be individually approved before publication |
| `diff_display_v1` | approved file identity/revision, bounded typed hunks, additions/deletions counts, truncation | existing workspace change authority and provenance; no raw private patch fallback |
| `search_display_v1` | bounded approved results with path/source identity, line, excerpt, total/truncation | source-level access checks and case evidence permissions remain with Go; UI cannot widen search scope |
| `web_display_v1` | validated HTTP(S) URL/title, bounded approved excerpt/citation descriptors | explicit network/retrieval permission, source attribution and publication approval; no credentials, raw response body or guessed citation authority |

These are proposed names, not current schemas. Displayable content does not
automatically become fact/evidence authority. Retained artifacts continue to use
their existing artifact/source receipts and authorized download owner.

Initial review budgets: 64 KiB per assistant segment, 32 KiB per tool excerpt,
100 search rows, 200 diff hunks/5,000 displayed lines, 2 MiB per publication batch.
The producer truncates deterministically with an explicit marker, never by
silently cutting a receipt. A larger source is exposed only through an existing
authorized artifact/file download, whose identity and permission are checked
again. These budgets require performance and security acceptance before adoption.

## Identity, lifecycle and compatibility

1. Go constructs the closed payload from approved outputs. Permission or receipt
   expiry/revocation removes pending segments and emits the existing revocation
   lifecycle; the renderer discards matching retained display state.
2. Append requires matching projection generation, previous revision and segment
   index. A reset/replacement has an explicit new generation. Duplicate events
   are idempotent; gaps suspend display and request the normal resync path.
3. Segments remain temporary display content until the final accepted projection
   is committed atomically. Finalization replaces the display exactly once and
   preserves message identity, measurement and reader scroll position.
4. Go owns durable event sequence, replay, compaction and accepted history.
   Renderer source offsets/fractional visual anchors never become durable seq.
   The same publication checks apply to live SSE and replay; ACK follows complete
   validated atomic-batch application, not receipt of a private/partial event.
5. A reconnect/rebind carries thread, turn, item, projection generation and
   workspace incarnation. Stale callbacks, files, image reads and downloads must
   not operate on the newly bound workspace. Existing local draft restoration
   stays independent of runtime history authority.

## Exact owner propagation and acceptance

Producer slice: Go `packages/runtime-go/internal/domain/toolresult/public_projection.go`,
`app/toolcatalog/public_projection.go`, `app/thread/public_event_projection.go`,
`domain/event/publication.go`, `app/loop/event_recorder.go` and
`adapters/inbound/sse/events.go` (`PreflightPublic` / `ProjectPublic`).
`privateAssistantTextDeltaEvent` remains private.

Consumer slice: TypeScript `contracts/items.ts` and event schemas → desktop
`src/shared/public-runtime-content.ts` / `public-runtime-sse.ts` → main runtime
`src/main/runtime/runtime-thread-subscription.ts` → `src/preload/index.ts` →
`src/renderer/src/agent/runtime-client.ts` / `src/renderer/src/store/chat-store-runtime.ts`
projection → existing active stream/timeline → the admitted presentation leaf.
Change only the required public slice after its design is accepted.

Acceptance must cover permission-denied and revoked content, version/kind/field
allowlists, malformed and oversized payloads, private sentinels in DOM/copy/
download/search/error/diagnostic sinks, stale workspace and generation, gaps/
duplicates/replay, atomic final receipt/ACK, compaction/resume and scroll-anchor
retention. Producer tests, IPC contracts and synthetic renderer evidence are
distinct from authorized live-provider, packaged/native and real-case acceptance.
Roll back through version negotiation to the existing metadata-only vocabulary;
never silently route to an alternate runtime or expose old private fields.
