# Threat Model

Status: Normative security attachment for this change.

## Protected Assets

- Integrity of case facts, citations, legal characterization, and reports.
- Isolation between tenants, users, threads, turns, cases, epochs, and datasets.
- Confidentiality of credentials, raw/restricted PII, provider reasoning, and case data.
- Auditability of source acquisition, transformations, claims, publications, approvals, and recovery.
- Availability without converting missing/partial evidence into fabricated certainty.

## Adversaries And Faults

| Threat | Example | Required control |
| --- | --- | --- |
| Malicious or confused provider | Emits amounts/accounts/MAC/relations without calls; forges a tool call; claims `safeToAnswer`. | Grant match, staged text, receipt-backed claim gate. |
| Prompt injection in data | CSV row or MCP text says to ignore policy or fabricate a report. | Data/protocol separation, no instruction interpretation, typed normalization. |
| Spoofed source | Fake server calls itself `analytix_funds`. | Configured identity proof, current connection epoch, case-bound health. |
| Stale source | Cached catalog exists after disconnect or case switch. | Live probe required to mint grants; cached catalog is schema-only. |
| Cross-case replay | Case A late result supports case B. | Frozen digest/epoch/snapshot checks at outcome, claim, final, and publication gates. |
| Citation confusion | Real receipt supports another subject/amount/date/direction. | Exact normalized field and supported-scope verification. |
| Empty/partial upgrade | `[]` becomes zero/no anomaly; one month becomes whole-case conclusion. | Coverage/completeness semantics and closed answer variants. |
| Recovery escalation | Failure prompt tells model to continue and invent facts. | Recovery cannot raise evidence state; unavailable is host terminal. |
| Pending-gate replay | Approval or user input resumes after case/epoch change or restart. | Persist digest/grant, revalidate before resume, reject stale. |
| Report fallback fabrication | Backend fails and local fallback inserts hard-coded values/case scenario. | No fact fallback, staged verified claims only, publication receipt. |
| Reasoning disclosure | Provider thinking enters SSE, disk, history, compaction, export. | Ephemeral reasoning sink and zero-byte leak tests on all paths. |
| Raw tool-result promotion | MCP/tool output carries full PII, prompt injection, arbitrary result codes, remote citations, `dataUrl`, URLs, paths, files, or diagnostics that become UI detail, media fetch/read/save, automatic preview, history, or export. | Private current-attempt result channel; exact root allowlist plus strict `PublicToolResultProjectionV1`; host-enumerated codes; separate Evidence Registry/controlled-artifact authority; HTTP/SSE/main/preload/renderer hostile-sentinel tests. |
| Tool-call argument persistence | Provider arguments contain commands, write bodies, credentials, case PII, prompt injection, or inline media and survive ordinary history/restart after the result is withheld. | Strict grant/schema validation before execution and a separate closed durable argument projection; raw argument bytes remain attempt-private/audit-bound and never become public replay authority. |
| Tool media authority spoof | A result supplies an attachment id, blob URL, local path, or screenshot-like payload and later code treats it as a readable artifact. | Reuse attachment owner/use receipts with exact TSC/grant/call/result/effect-lease binding; public surfaces accept only typed controlled-artifact handles and never inline bytes or paths. |
| PII overexposure | Full account/id/phone copied to chat/log/report. | Default masked projections; controlled artifact authorization/audit. |
| Renderer artifact bypass or raw-memory PII | Case rows are turned into a Blob/download/clipboard/file before publication, or the ordinary renderer retains reversible raw account/card values behind masked labels. | P0 removes every case-artifact renderer download/write path; P2 sends only host-produced masked/opaque ordinary projections and resolves full PII through an audited host handle plus `PIIProjectionGrantV1`/`PublicationReceipt`. |
| Controlled-sink spoof, replay, forged receipt, or ambiguous acknowledgement | A local process impersonates Electron main, a renderer reuses a slot, the runtime presents a structurally valid or self-signed receipt from an untrusted key, a redirect/proxy captures bytes, a host restart accepts an old generation, or a connection drops after a partial/durable write. | Separate per-launch secret, exact loopback origin, no proxy/redirect, closed framed protocol, current main-frame and host-generation admission, invocation-frozen installation key plus independent Electron Ed25519 receipt/digest verification, one-use slot, atomic host effect, exact-length HMAC-bound acknowledgement, no automatic retry, and conservative `release_indeterminate` settlement after every ambiguous sink call. |
| SQL abuse | DuckDB query bypasses policy via comments/encoding/AST shape. | Parser/AST read-only policy, timeout, row/byte limit, deterministic diagnostics. |
| External data runtime substitution or resource abuse | A converter/extractor executable or cwd is symlink-swapped, stdout is unbounded, a child escapes cancellation, an archive bomb exhausts disk, a stale marker bypasses hashing, or a predictable temp path overwrites a victim. | Canonical regular executable/cwd identity, closed environment, output/time/tree bounds, stable-handle full SHA-256, archive member/byte/path/depth quotas, no-overwrite staging, fsync, atomic publish, and hostile race/flood/bomb tests. |
| Registry corruption, local rollback, or authority ambiguity | Receipt id inserted without a valid chain; risk/evidence head or per-turn capsule rolled back; complete old private-state snapshot restored; foreign/forked capsule; root predecessor replaced; non-canonical/duplicate-key JSON; interrupted CAS/index/witness commit; or legacy MCP identity replayed as live authority. | Installation-key-signed immutable records and exact predecessor transitions prove integrity, while an independent exact-CAS monotonic witness proves the current risk/evidence authority generation and digest. Local head files are projections only. Startup and effect/publication boundaries compare live witness state; deletion, composite old-state rollback, equivocation, indeterminate commits, missing referenced records, or witness failure quarantine high-risk work to host boundary-only. Current `mcpv2` identity remains required throughout grant/outcome/receipt membership. |
| Rollback bypass | Old release restores free-text case answers. | Gate-preserving rollback only; otherwise boundary-only mode. |
| Concurrent/aliased runtime writers | Two processes or case/symlink aliases mutate the same durable/private authority roots. | Canonical non-overlapping roots, case-insensitive-volume lease keys, hierarchical composite process lease, frozen-root constructor binding, production factory ownership, strict raw snapshot, and full startup readback. |
| Startup repair before validation | One repairable committed final is appended before a later corrupt log is discovered. | Inventory-wide dry-run, synthetic complete replay validation, then append-only idempotent repair and global readback. |
| Partial accepted-final visibility | A crash after the first of several final events leaves a visible factual prefix. | Whole-manifest validation, one target-native atomic event-log replacement, committed-state readback after barrier ambiguity, orphan-temp rejection, and SSE emission only after complete durable commit. |
| Runtime token disclosure or implicit insecure mode | A bearer token reaches argv/READY/logs/untrusted child processes, or an empty cold-user token disables authentication. | Per-launch CSPRNG desktop token, immediate host-environment removal, sanitized child environments with reserved-key rejection, closed boolean-only READY, explicit loopback-only insecure opt-in, and standalone fail-closed token validation. |
| Native build resumes before authority is proven, loses a response, or leaks a source generation | A suspended helper is resumed by the wrong actor; output/reap/rehash fails after effects; create/discard commits but its response is lost; cleanup reuses a caller generation id; or a user-owned toolchain self-attests release eligibility. | Per-function gate poison and stable post-resume indeterminate errors; close-once FD cleanup; separate source-read and two-FD cleanup profiles; three-digest exact discard; identity-pinned `reconcile_discard` that can only remove empty/one-clean-current/already-detached state; host-derived Cargo execution eligibility; and a non-serializable one-use publication permit. Indeterminate execution never becomes probe or publication evidence. Current same-UID and process-tree-containment gaps keep the authority `local_provisional` and release-blocked. |
| Cross-architecture, signing-mutated, or unregistered native helper | An x64 package reuses a stale arm64 helper, signing invalidates or bypasses a raw receipt, a symlink/override bypasses the registry, or a new Rust/Tauri Agent Runtime hides under an approved data-tool directory. | Frozen four-component registry, global Tauri denial, explicit Cargo target, locked fail-closed stage publication, raw-build plus signing-invariant payload receipts, strict Mach-O/PE/ELF parsing, post-sign platform verification and live probes, target-only Electron/Python resolution, one frozen Go execution authority, and zero alternate Agent Core or direct Electron/Python helper execution. |
| Mutable packaged interpreter or forged signature verifier | A local Python override self-issues its manifest, floating wheels change between releases, packaged Python/site files are replaced, path-based `codesign` observes swapped bytes, a fake verifier reports success, or an old same-signer helper is paired with a forged receipt. | Electron/plugin execution remains P0-quarantined; fixed archive/tree/wheel hashes remain package evidence only. Darwin verification starts the exact staged vnode suspended, binds the loaded image by full SHA-256 and CodeDirectory hash, targets `codesign` by numeric PID, revalidates afterward, and kills/waits without resume. Future Windows execution additionally requires Go-owned no-breakaway Job Object containment and OS-rooted signer pinning. |

## Security Properties

1. No accepted high-risk claim without a matching registry receipt.
2. No receipt crosses its context digest or dataset snapshot.
3. No provider call executes outside its current grant.
4. No terminal path bypasses finalization.
5. No unavailable/partial/empty state is upgraded by recovery or wording.
6. No formal report exists as an accepted publication without a publication receipt.
7. No provider reasoning is emitted or stored outside ephemeral current-call protocol memory.
8. No raw tool-result or root-level covert field reaches an ordinary public, replay, UI, action, report, or export surface.
9. No tool-originated media is readable without a current host owner/use authority and effect lease.
10. No complete controlled-PII byte reaches ordinary runtime HTTP/SSE, preload, renderer, or an unauthenticated/redirected sink, and no sink call is recorded committed without an exact host-authenticated durable acknowledgement.

## Abuse Tests

The deterministic red-team suite includes direct fabricated facts, forged calls, fake identities, semantic failures, empty/partial outcomes, fake/mismatched citations, case switches, stale approvals, restart replay, prompt injection, every terminal, report fallback, `write_report=false`, provider families, and the verbatim incident. Tests assert both rejection and absence from SSE/disk/UI/history/export.
