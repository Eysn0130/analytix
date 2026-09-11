# Case Evidence Publication Architecture

Status: Normative design attachment for this change.
Source of truth after implementation: current code, versioned contracts, and fresh tests.

## Trust Boundary

The host trusts only its own canonical context, grant registry, evidence registry, verifier, and accepted publication events. User text, provider output, CSV/database values, MCP responses, cached catalogs, model-reported identifiers, file existence, and rendered artifacts are untrusted inputs.

## Main Chain

```text
desktop request
  -> runtimeapp turn-start boundary
  -> strict host binding observer classifies valid/missing/invalid/unreadable/unstable
  -> signed thread-risk index is compared with the live monotonic witness
  -> host snapshot authority selects one immutable dataset snapshot
  -> context app service freezes executable TurnSecurityContextV2 or boundary-only V2
  -> source readiness app service performs live native probes and verifies the exact frozen snapshot
  -> grant app service issues ExecutionGrant records
  -> provider may propose only advertised granted tool calls
  -> tool adapter executes and returns raw ToolOutcome material
  -> outcome verifier checks schema, identity, hashes, status, scope, epoch
  -> evidence registry issues and persists EvidenceReceipt
  -> claim normalizer creates ClaimRecord proposals
  -> claim verifier resolves support/counterevidence/source capability
  -> terminal finalizer builds a closed FinalAnswerEnvelope
  -> deterministic renderer produces accepted answer/report material
  -> accepted event is atomically persisted then projected through SSE/UI
```

Accepted-final event publication uses one host-owned per-thread/per-commit
reservation from durable staging through live delivery. Ordinary, raw,
checkpoint, and general-terminal writers cannot acquire a same-thread
sequence while the reservation exists. The installation authority may sign a
staged delivery only after an exact full-log readback proves that the unique
manifest is the physical tail; replay signing proves the same unique durable
manifest but does not require it to remain the tail. Every fallible delivery
check completes before trusted projection activation. Activation and the
in-memory publication-marker/batch broadcast then execute under the same
reservation as a no-I/O, no-error commit. A durable pre-activation failure
retains the commit reservation for exact retry or startup recovery, so a later
event cannot make the accepted final unreachable by cursor.

The reservation, atomic stage, durable readback, prepared-delivery validation,
activation, and live commit are owned by one
`adapters/outbound/acceptedfinalevent` instance. It is composed with the
durable event store's existing owner mutex; it does not create a second lock or
copy reservation state into `internal/server`. `internal/server` retains only
ordinary-writer reservation checks and its subscriber broadcast callback.
The application sees one `ports/acceptedfinalevent.Delivery`; legacy direct
stage/publish/prepare server methods and the unreserved publish helper do not
exist.

The sealed live delivery itself has one current closed profile. Its three-slot
form is `assistant-final -> usage -> terminal`; its four-slot form inserts one
host-authored `terminal-error-item` after the assistant. The terminal reason
selects the exact status, event kind, and slot count; usage status must equal
terminal status. Failed and aborted reasons, plus `approval_denied` and
`input_cancelled`, require the four-slot form. The error event/item identity,
thread, turn, commit digest, status, code, timestamp, severity, and terminal
item reference are equal-bound. Only aborted terminals carry discard/cancel
disposition fields. Go validates this closure before authority signing, the
public TypeScript schema repeats it at every desktop boundary, and Electron
main repeats it before accepting a pinned signature. A sealed batch is never
sanitized or rewritten before verification; it is either preserved exactly or
rejected in full.

Electron main retains the exact verified batch identity until the renderer has
atomically applied it. A terminal acknowledgement binds stream id, last
sequence, batch id, thread, turn, and publication commit; an ordinary
sequence-only acknowledgement cannot release that waiter. The terminal
waiter has no success timeout: it ends only on the exact acknowledgement,
explicit stop, or destruction of its owning WebContents. One owner-level
destroy observer cancels every active response reader and aborts its waiters,
so a connected body or a slow renderer cannot split transport acceptance from
UI acceptance. The browser preview has no accepted-final authority and rejects
the extended acknowledgement form.

The renderer cannot construct that acknowledgement from raw SSE fields. Its
main and side stores first commit the accepted assistant, optional terminal
error, usage, terminal disposition, cursor, and a versioned projection receipt
in one state transition. The sink returns that exact receipt to the SSE
consumer, which checks every ACK binding field before acknowledging. A first
commit requires the exact stable turn id and `firstSeq - 1` predecessor;
`pending:*`, gaps, partial ranges, and later cursors fail closed. Replay is
idempotent only when one receipt-bearing assistant, the optional error item,
usage, terminal state, cleared live state, and `lastSeq` all match exactly.
Torn or mismatched same-turn state is quarantined and never repaired into
authority. Receipt-bearing side projections are purged by public-authority
revocation, and accepted-final batches cannot share a renderer dispatch with
ordinary events.

## Failure Chain

```text
unavailable | failed | partial | stale | spoofed | unsupported | unknown
  -> typed blocker and rejected/narrowed claims
  -> SourceUnavailableAnswer | NeedsEvidenceAnswer | PartialEvidenceAnswer
  -> host template
  -> accepted boundary-only event
```

No failure chain asks a model to invent a replacement case conclusion.

A case-risk request whose binding or monotonic authority is not executable takes a shorter chain: host observation -> signed boundary-only V2 -> Final Evidence Gate -> deterministic boundary renderer -> accepted event. Provider configuration, attachment/vision resolution, MCP, research, shell, jobs, and report generation are not invoked.

### Attachment Owner And Use Authority

An attachment id is an upload-owner identity, not a global blob capability. Upload requires an existing host thread; the host resolves the thread's canonical workspace and ignores caller-reported scope. Each upload receives a distinct `AttachmentOwnerRecordV1`. The record binds the owner id, random owner nonce, full blob SHA-256, exact byte size, MIME type, thread, canonical workspace, optional current valid case-binding observation, provider-visible projection SHA-256, and creation time. Equal bytes uploaded in two owners or cases never merge metadata, paths, extracted text, fallbacks, or authority. Internal blob deduplication is permitted only behind separate owner records and may not change their authorization semantics. Production upload is one host use case: a strict private `UploadIntentV1` is committed in the existing attachment-authority `SecurePrivateCAS`, exact content and metadata become durable, the live thread/workspace/case binding is rechecked, owner membership is committed atomically against the still-open intent, and a mutually exclusive `UploadDispositionV1` readback precedes HTTP 201. `committed` requires the exact owner and file digests; `rejected` or `quarantined` forbids owner membership.

Legacy/global/workspace-only/unknown-scope records, owner-id aliases, missing requested ids, or records whose owner, projection, size, MIME, or full content hash fails exact validation are non-executable. A request containing any such id fails the whole attachment admission before vision or primary provider effects; it never degrades into a model request with the attachment silently omitted. Ordinary upload/metadata/content responses and durable turn/event projections contain only bounded display metadata. `documentText`, `textFallback`, raw bytes, blob hashes, caller-reported `localFilePath`/`FilePath`, owner records, and case observations remain private input material. The renderer never copies extracted document text into a user-message prompt; the Go host constructs provider-only attachment parts after authorization. Public queues, optimistic blocks, SSE, history, compaction, and exports use a closed attachment display projection. Uploaded attachments and host-verified generated artifacts are different authority types and may not merge by filename.

`AttachmentOwnerRecordV1` alone is not turn execution authority. Before reading private bytes for a provider, the host must hold the current context-effect lease and issue a private `AttachmentUseReceiptV1` bound to the exact `threadId`, `turnId`, `caseId`, `caseBindingHash`, `contextEpoch`, `datasetSnapshotId`, `contextDigest`, owner digest, blob SHA-256, and projection SHA-256. A case switch, epoch/snapshot change, restart-indeterminate use, missing receipt, or mismatched owner blocks use and closes through the Final Evidence Gate. Startup performs one read-only owner/intent/disposition/file inventory before recovery mutations. An open intent can gain owner authority only when both files and all hashes are exact and the live binding still matches; owner-before-response recovers only to committed, while partial, corrupt, missing, stale, legacy, or unattributed material remains non-executable quarantine. The signed use receipt, post-durable-TSC effect ordering, and upload transaction are composed in `runtimeapp`; P1 remains open for the full desktop public attachment contract, controlled-artifact access, legacy migration, platform hardening, and complete crash/fault matrix.

Tool-originated screenshots, images, audio, generated files, and other binary results reuse this owner/use/effect-lease authority; they do not create a parallel screenshot receipt. Their private origin additionally binds the frozen `TurnSecurityContext`, current `ExecutionGrant`, tool call, result item, result hash, and provider attempt. Bytes may be materialized only inside that current attempt while its effect lease remains valid. Ordinary SSE, history, UI, compaction, and export receive neither inline `dataUrl`/blob URLs nor raw/local/absolute paths. A future public media surface accepts only a separately typed, host-issued controlled-artifact handle whose owner/use authority is revalidated on access.

## Versioned Values

All values include `schemaVersion` and canonical JSON/hash rules. IDs use host-generated random identifiers; integrity comes from registry membership and hash linkage, not id format.

### TurnSecurityContextV2

| Field | Authority/invariant |
| --- | --- |
| `threadId`, `turnId` | Current durable runtime objects. |
| `workspaceRealPath` | Host `realpath`, never model/MCP text. |
| `tenantId`, `userId` | Authenticated host identity or explicit local identity. |
| `caseId` | Resolved binding at turn start. |
| `caseBindingHash` | Full SHA-256 of canonical binding material. |
| `datasetSnapshotId` | Host-derived `dsv1_` content identity for the immutable snapshot selected before TSC minting. |
| `sourceManifestHash` | Full SHA-256 of the snapshot's canonical immutable source manifest; transport connection/health state is not substituted for dataset provenance. |
| `contextEpoch` | The single accepted Context Epoch. |
| `issuedAt` | Host time. |
| `publicationPolicy` | Host-derived general, case-evidence, or boundary-only policy bound to the exact signed thread-risk policy and binding observation. |
| `riskAuthorityBinding` | Closed nested witnessed/quarantined union. A witnessed value binds exact index digest/generation, checkpoint digest, and fresh observation digest selected by the enrolled witness; a quarantined value carries no authority references and is boundary-only. |
| `contextDigest` | SHA-256 of canonical versioned fields above. |

V1 remains strict audit/migration input only. It cannot authorize provider, tool, grant, receipt, job, continuation, final, report, or compaction execution.

An executable case-evidence V2 contains a concrete host-registry snapshot id. `unresolved:*`, MCP-reported snapshot selection, cached catalog data, or a provider/tool claim cannot authorize execution. The live probe is bound to the already frozen context and must report the exact same snapshot; it may downgrade readiness or force a later epoch transition, but it never mutates, upgrades, or remints the current turn context.

`sourceManifestHash` identifies immutable dataset provenance (including the source set/material that produced the snapshot), not the current MCP socket, connection epoch, or cached readiness. Current-run transport authority remains separately bound by the live probe, verified server identity, connection epoch, fresh catalog/spec fingerprints, `ExecutionGrant`, and later receipt. Reconnecting an identical source cannot silently change snapshot provenance; changing raw material, accepted/rejected rows, parser/transformation version, or the canonical dataset source manifest necessarily creates a new host-derived snapshot id and advances the shared Context Epoch.

### Monotonic Authority Witness V1

Thread-risk and Evidence Registry heads use separate versioned namespaces over one generic protocol. A stable signed checkpoint records installation/enrollment, namespace, generation, current and previous state digests, predecessor checkpoint, and a witness fencing token. A separately signed observation echoes a fresh client challenge, preventing replay of an older valid checkpoint. Advance uses an installation-signed exact expected-generation/checkpoint/state compare-and-swap plus a deterministic mutation id; retry returns the byte-identical receipt.

Local policy/index/intent/receipt records are immutable CAS material. A mutable local head is only a projection and may be repaired solely from a fresh witness observation. Witness unavailability, signature/enrollment/nonce mismatch, same-generation equivocation, a witness head whose referenced local material is missing, an indeterminate advance, or a local projection ahead of the witness quarantines high-risk execution. Signatures, maximum generations, Keychain/DPAPI/libsecret, or self-consistent old snapshots are not freshness proofs. Witness payloads contain digests and installation metadata only, never thread/case/workspace/prompt/evidence/PII content.

### Authority Advance Journal V2

The witness protocol remains V1, but its local write-ahead journal is a separate cryptographic wire contract. `MonotonicAdvanceIntentV1` and `MonotonicAdvanceSettlementV1` are frozen successor-only audit/migration records. Enrollment-to-generation-one thread-risk and evidence-authority genesis, successor transitions, committed settlements, superseded settlements, and versioned range references use V2 schema/purpose values plus V2-only signature and digest domains. A V2 envelope around an expanded V1 payload is forbidden.

New writers use only `<authority-journal>/v2/{intents,settlements}` and one 16 MiB domain/adapter record bound. The host-owned immutable keyed store treats `MutationID` as a canonical caller key, not as `SHA256(body)`; signed record validation binds the key to canonical bytes. Root, shard, record, and temporary objects require handle-relative no-follow access, current-owner private permissions/ACL or protected DACL, single-link records, stable multi-read identity, canonical inventory recheck, target-native no-replace commit, and exact committed readback. Local inventory never selects current witness state.

The same host-authorized `SecurePrivateCAS` owns accepted-final, pending-work, case-thread, and later admitted immutable authority stores. Callers carry an `AccessAuthority`, frozen root binding, and lease through `Put`, `Read`, and streaming `Visit`; callbacks are provisional until final root/inventory revalidation succeeds. A single typed topology catalog owns every fixed directory, physical CAS root, shard parent, recoverable owner group, and body policy. Startup first performs a no-create, complete repeated observation that may remove only exact, strictly empty staged-directory create residue and can never delete canonical final topology. It then resolves the already signed semantic journal, performs a fresh no-create preflight for empty orphan shards and recursively empty incomplete owner topology, completes one read-only preflight across every currently composed CAS owner, performs store-owned recovery, and only then seals the first generic baseline. This order preserves an older signed plan that explicitly removes a case-thread CAS residue while preventing a pre-journal directory cleanup from acquiring authority over canonical final state. Such a residue is admitted to journal recovery only when the authenticated plan contains the exact canonical `remove_file` transition with its before hash/mode/size; ordinary snapshots still reject it. Missing CAS roots remain absent. The generic persistence snapshot is not a second CAS recovery implementation and rejects every unapproved residual CAS temp/hardlink. Startup filesystem scans and deletion are paged, resource-bounded, repeatable from independently reopened directory handles, and complete-preflight-before-mutation. Unix deletion retains and revalidates the exact directory identity through name removal; Windows deletion additionally holds a final exact-name handle that does not share delete access until the name is absent and its parent is flushed. An architecture test discovers CAS-backed constructors transitively and requires each runtime-composed owner package to register both preflight and recovery.

The process cancellation context is installed before persistence preparation and is propagated through journal recovery, CAS preflight/recovery, both baseline captures, stage copy/simulation, apply, final generation readback, and activation. A signal before activation cannot open the listener or publish readiness. Resumable journal/tombstone state, rather than an unsafe rollback, owns any cancellation that occurs after a durable mutation boundary.

An unresolved V1 writer may not race a V2 writer. V2 activation therefore requires a separately journaled fixed point proving that every V1 intent is terminal, no V1 writer lease remains, the V1 writer is disabled atomically with V2 activation, and a fresh live witness observation fixes the exact tail checkpoint. Until that marker and the complete Observe/range/startup reconciliation path validate, Authority Advance V2 remains uncomposed in `runtimeapp`, `RecoverExact` does not replay an unresolved remote mutation, and high-risk work stays boundary-only.

### ExecutionGrantV1

`grantId`, `turnId`, `contextDigest`, `provider`, `serverIdentity`, `toolName`, `schemaHash`, `scopeHash`, `readOnly`, `approvalState`, `expiresAt`, and registry linkage. Approval state is checked again immediately before execution.

### ToolOutcomeV1

`transportStatus`, `semanticStatus`, `safeToAnswer`, `isError`, `blocker`, `caseId`, `contextEpoch`, `partialCoverage`, `data`, and candidate evidence material. These external fields are parsed but do not override host context.

### PublicToolResultProjectionV1

Tool results have three non-interchangeable channels: private current-attempt result material, host-authoritative Evidence Registry/private-CAS material, and a closed ordinary-public projection. `PublicToolResultProjectionV1` is a strict metadata-only discriminated union (`withheld`, `host_status`, `plan_status`, `case_source_status`, or `mcp_diagnostic`). Every variant fixes `privatePayloadWithheld=true`, `factAnswerAllowed=false`, and `evidenceAuthority=false`; variant codes and JSON-RPC diagnostics come from host enums, not arbitrary tool fields. The outer public item is an exact allowlist bound to its tool/call and, for case outcomes, the same context digest, epoch, and execution grant. Unknown fields, root-level summaries/details/media, raw output, mismatched bindings, and legacy/open shapes mechanically become `legacy_output_withheld` or are dropped.

Private result bytes may be used for the immediate provider continuation only while the same attempt and effect lease remain valid. They are never reconstructed from a public projection and never enter ordinary thread/events/messages, provider replay history, compaction, search, fork/resume, logs, UI, report, or export. Controlled evidence and artifacts remain separate typed authorities; a public id or path does not acquire authority from this projection.

Host-native capability probes reuse the same durable `tool_call` / `tool_result` grant registry rather than creating a second health registry. The coordinator acquires the current context-effect lease, appends one deterministic host-only call without a thread event, re-reads exact active membership, executes through the sole native runner, appends a metadata-only result with `factAnswerAllowed=false` and `evidenceAuthority=false`, and returns ready only after durable `active -> settled` readback. Raw registry digests, paths, process identities, stdout/stderr, and runner errors remain attempt-private. A turn may contain at most one such probe, and native readiness never substitutes for MCP identity, case-bound source health, dataset coverage, or an EvidenceReceipt.

### EvidenceReceiptV1

`receiptId`, `contextDigest`, `toolCallId`, server identity/version, `connectionEpoch`, `toolName`, `argsHash`, `resultHash`, source type, `datasetSnapshotId`, `queryHash`, range, granularity, currency, timezone, pagination completeness, `sourceRecordIds`, raw SHA-256, transformation lineage, PII classification, `issuedAt`, and registry integrity proof.

The registry's membership authority is an installation-key-signed global root index whose ordered entry binds one turn/context to the full SHA-256, byte length, record digest, sequence, state digest, last-entry digest, and canonical-ledger digest of a content-addressed authority capsule. Each capsule binds the complete frozen `TurnSecurityContext` and current registry. Index generations bind `previousIndexDigest`; a live update may add a sequence-one context or extend exactly one existing registry by one canonical entry, while every other entry remains unchanged. A capsule blob is durably installed before the root index is atomically replaced; that index replacement is the local membership commit point. The per-turn capsule and JSONL files are disposable projections: missing or complete-record exact prefixes may be repaired, while longer, mid-record, divergent, wrong-key, unindexed, or non-canonical state fails closed. Superseded capsule blobs are removed after commit, and any retained orphan must be an exact historical prefix; a newer or divergent installation-signed orphan is local rollback evidence. The current root index is additionally advanced through the Evidence Registry monotonic-witness namespace. Receipt settlement/revocation, accepted finals, compaction references, and PublicationReceipts bind that witness checkpoint, so restoring the complete older local registry cannot resurrect support authority.

MCP fact authority uses Streamable HTTP/stdio revisions `2025-11-25` or `2025-06-18`. The negotiated revision is frozen into host-issued `mcpv2` server identity and therefore into `ExecutionGrant`, source probe, `ToolOutcome`, and `EvidenceReceipt`. The 2024 HTTP+SSE revision and legacy `mcpv1` identities remain audit-only and cannot authorize new facts.

### ClaimRecordV1

`claimId`, `claimType`, `normalizedPayload`, `verified|partial|unresolved|refuted`, evidence and counterevidence ids, supported scope, allowed wording, prohibited upgrades, human-review requirement, and verifier receipt id.

### FinalAnswerEnvelopeV1

Closed variants: evidence-backed, partial evidence, verified no-hit, source unavailable, needs evidence, and general guidance. Unknown variants reject at external boundaries.

### PublicationReceiptV1

`publicationReceiptId`, context digest, report hash, claim-ledger hash, dataset snapshot, PII projection hash/class, render inspection result, publisher version, atomic target identity, issued time, and registry proof.

### ControlledArtifactAccessReceiptV1

A committed controlled-PII publication is not itself display/export authority. The desktop host resolves an opaque publication handle, one-use slot, exact main-frame principal, renderer generation, backend generation, action, and expiry into a host admission; caller-supplied hashes or generations are never accepted as authority. Under the same context-effect lease, Go revalidates the frozen case context, `PublicationCommitReceiptV1`, candidate `PublicationReceiptV1`, publication index, claim ledger, controlled `PIIProjectionV1`, current `PIIProjectionGrantV1`, render inspection, and exact protected artifact bytes. It durably reserves and reads back a signed private access receipt before any byte reaches a trusted host sink. A second use of the slot fails closed.

Complete account/card values remain only in the protected artifact and the synchronous trusted-sink call. The access result, receipt, disposition, SSE, UI, logs, history, compaction, and ordinary exports contain hashes and bounded metadata only. A pre-sink stale/rejected/cancelled access closes with zero released bytes. Once the sink is called, only an exact-length synchronous commit plus post-release authority revalidation is `host_release_committed`; partial, failed, expired, cancelled, or ambiguous outcomes are durably `release_indeterminate` with the complete artifact length as the conservative exposure upper bound. Restart closes every still-open reservation the same way before listener activation. Generic renderer paths, save targets, blob downloads, and arbitrary write IPC remain quarantined.

The production trusted sink is a dedicated Electron-main authority, not the ordinary runtime HTTP response or renderer bridge. Main starts an ephemeral `127.0.0.1` listener before launching Go and passes a separate 256-bit CSPRNG bearer secret and the exact loopback origin through reserved child environment fields. Go validates the origin, copies the configuration once, and removes both fields from its environment before serving. The client disables redirects and proxies, pins loopback, applies strict time/body limits, and accepts only closed versioned admission and release-ack schemas. The host compares the opaque handle, use slot, action, current main-frame principal, renderer generation, host generation, publication commit, frozen thread/turn context, and expiry against its in-memory registry on every admission call; caller-provided digests or generations never create authority. Invocation creation also freezes the exact installation authority key. Before any display/export effect, Electron main independently reconstructs the canonical Go receipt bytes, verifies the Ed25519 signature and record digest, and requires the receipt key to equal that frozen authority. A syntactically valid or self-signed receipt from another key releases zero bytes.

Release uses a length-delimited versioned metadata frame followed by the exact artifact bytes. Main revalidates the same registry entry immediately before the side effect, writes an export through host-owned no-follow staging and atomic replacement or publishes a protected display copy before invoking the OS viewer, then consumes the use slot. It returns an HMAC-authenticated acknowledgement bound to the access-receipt digest, artifact SHA-256, exact byte length, action, handle digest, use-slot digest, and host generation only after the side effect is durable. Go accepts no boolean-only acknowledgement. A connection loss, malformed or unauthenticated acknowledgement, partial write, viewer failure, generation change, duplicate call, timeout, or process restart after the sink call is indeterminate. Renderer IPC receives only a fixed status and bounded audit metadata; raw tokens, target paths, host secret, artifact bytes, and complete identifiers never cross preload or renderer memory.

## Persistence And Projection

- Raw provider proposals remain ephemeral until finalization.
- Raw `ToolOutcome.data` and arbitrary tool/MCP result payloads stay private to the current effect/provider attempt. Durable ordinary tool-result items use only `PublicToolResultProjectionV1`; verified receipts and controlled artifacts persist in their separate host-authoritative stores.
- Accepted final envelopes are the only assistant facts available to thread history and compaction.
- Legacy assistant events remain displayable with an unverified marker but are excluded from evidence/history composition for new claims. Legacy `output:any` is read-time contained as `legacy_output_withheld` and still requires a journaled migration before P1 closure.
- SSE encodes host progress, gated tools, receipts (bounded metadata), blockers, and accepted final envelopes. It does not encode reasoning or rejected drafts.

Electron has no background-task registry, process shadow, or task-output
authority. Task identity, status, output, kill, restart, and resume originate
only from the Go thread-summary/task routes and their frozen runtime authority.
Until the compatibility IPC is removed from every public consumer, it is a
stateless fail-closed facade: list/snapshot are empty, register/kill/restart are
retired, and output returns one closed withheld variant without echoing a task
id, status, byte count, or any caller/storage field. The facade never opens,
parses, normalizes, or rewrites `background-tasks.json`.

Historical Electron task storage remains an opaque migration input in the
Electron user-data root, which is distinct from the Go data and durable roots.
Its owner-specific inventory binds strong file identity, type, size, mode, and
streaming full SHA-256 without decoding JSON or projecting alleged metadata.
It participates with Go `child-runs` and every other security-relevant root in
one composite read-only fixed point and shared activation barrier, but not in a
fictional shared `persistencefs.RootSet` or single-owner journal. Only after all
owner plans validate may an owner journal authorize monotonic removal and
durable absence readback. No Electron-local startup rewrite may claim to remove
historical reasoning or PII bytes.
- Renderer projection treats an accepted final envelope as the authority, ignores provider text not linked to that event, and never promotes arbitrary tool output into detail, citations, media, file access, generated files, or automatic preview actions.

## Cache Keys

Every factual cache key contains at least tenant, user, thread, case, binding hash, context epoch, dataset snapshot, source identity/connection epoch, tool/query/scope hash, and schema/parser version. Literal or semantic aliases such as `active`, `current`, or last-opened are forbidden factual keys.

## Context Epoch Merge Boundary

The accepted `context-epoch` capability from archived `context-epoch-foundation` owns generic registry/snapshot/reconcile/recovery. This change consumes its accepted snapshot when minting `TurnSecurityContext` and adds case binding/dataset sources. Both scopes use the same domain types and durable thread metadata. The foundation does not issue evidence or publish claims; this change does not create an independent epoch lifecycle.
