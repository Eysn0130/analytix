## 1. K1 — Protected Secret Store

- [x] 1.1 Implement the versioned authenticated-encryption envelope, opaque credential identifiers, purpose-bound associated data, atomic persistence, and redacted error model for Provider and token secrets.
- [x] 1.2 Implement operating-system master-key protection and the restrictive exclusive-create fallback, including concurrent-winner re-read and fail-closed permission or key mismatch handling.
- [x] 1.3 Implement Secret Store put, get-for-authorized-consumer, replace, tombstone, and explicit-delete primitives without exposing a general secret-read API to renderer or public IPC.
- [x] 1.4 Add focused tests for encryption round trips, nonce uniqueness, AAD mismatch, tamper detection, unreadable keys, concurrent key initialization, restrictive fallback permissions, atomic write failure, and delete/tombstone behavior using synthetic secrets.
- [x] 1.5 Complete K1 only when committed secret bytes remain encrypted and recoverable, public diagnostics are redacted, unreadable state fails closed without overwrite, and no second credential store is introduced.

## 2. K2 — Registry, CAS, fences, transactions, recovery, and local API

- [x] 2.1 Define the versioned Provider Registry metadata schema for Provider identity, endpoint/proxy/model state, selected routes, opaque `credentialRef`, revision, generation, incarnation, tombstone, and durable transaction records.
- [x] 2.2 Implement data-directory-scoped Registry locking and revision compare-and-set for connect, update, select, credential replace, disconnect, and explicit delete operations.
- [x] 2.3 Implement credential fences plus prepare/commit/rollback ordering so metadata never commits an unavailable candidate and superseded credentials are not cleaned before the winner is durable and verified.
- [x] 2.4 Implement idempotent restart recovery for prepared candidates, committed transactions, stale writers, tombstones, orphan candidates, generation/incarnation changes, and interrupted cleanup.
- [x] 2.5 Implement the main-owned local Provider API and key-free preload/public contracts for list/get/connect/update/select/disconnect/delete/credential-replace/recovery operations with redacted conflict and failure results.
- [x] 2.6 Add deterministic concurrency and crash-point tests covering competing writers, stale revisions, prepare interruption, post-commit interruption, cleanup replay, explicit deletion, and preservation of the last committed usable credential.
- [x] 2.7 Complete K2 only when every credential-bearing mutation is fenced and CAS-protected, recovery is idempotent and loss-resistant, and the local API cannot return credential bytes.

## 3. K3 — Key-free settings and legacy plaintext migration

- [x] 3.1 Remove credential-bearing fields from canonical settings, renderer snapshots, public IPC DTOs, serialization, logs, telemetry, diagnostics, and ordinary settings import/export while preserving key-free Provider/model preferences.
- [x] 3.2 Route settings and form updates through Registry operations so omitted, empty, masked, or redacted credential input preserves the committed secret and only explicit delete/disconnect can remove it.
- [x] 3.3 Implement transactional migration of supported legacy plaintext Provider settings and legacy Provider metadata into the Secret Store and Registry, with validation and encrypted readback before plaintext removal.
- [x] 3.4 Implement protected recovery records and rollback/finalization for migrated credentials so interruption or an incompatible input preserves a recoverable source without leaving plaintext in ordinary settings.
- [x] 3.5 Add focused fixtures and crash/restart tests for each supported legacy shape, malformed or conflicting input, repeat migration, redacted resave, rollback, finalization, and secret-free output auditing.
- [x] 3.6 Complete K3 only when canonical settings and every public surface are key-free, all supported legacy credentials migrate or fail closed without loss, and destructive redacted-empty updates are impossible.

## 4. K4 — Shared data directory, Manager, Registry, and runtime composition

- [x] 4.1 Compose one profile data directory, Manager, Secret Store, Registry, and local Provider API lifecycle in Electron main for both unpackaged development and packaged execution.
- [x] 4.2 Remove development-only or environment-backed product credential authorities so development uses the same Registry/Secret Store behavior with an explicitly isolated data directory.
- [x] 4.3 Resolve the selected committed Provider and credential just in time across the existing main-to-Go runtime boundary while keeping `packages/runtime-go` as the only production Agent runtime and provider egress owner.
- [x] 4.4 Implement revision/generation invalidation so runtime restarts and bounded Provider caches re-resolve or fail closed after update, revoke, disconnect, recovery, or authority change.
- [x] 4.5 Add lifecycle tests for first composition, restart, concurrent Manager attempts, isolated profiles, development/package configuration parity, data-directory separation, and stale-runtime resolution.
- [x] 4.6 Complete K4 only when development and packaged builds share one data-directory-scoped authority and the existing Electron and Go runtime architecture has no parallel Manager, Registry, credential path, or runtime.

## 5. K5 — First setup, startup gate, Hub cold boundary, and sidebar Settings

- [x] 5.1 Rework `InitialSetupDialog` and its save flow to commit credential and Provider state through the Registry transaction before persisting key-free settings or marking initial setup complete.
- [x] 5.2 Replace ordinary Hub/account readiness gating with local selected-Provider readiness, including fresh setup, ready workspace entry, and redacted recovery guidance for unusable local state.
- [x] 5.3 Remove ordinary startup Hub imports, service construction, refresh timers, token reads, account/model requests, and gateway selection; place preserved login and migration behavior behind an explicit lazy deprecated compatibility boundary.
- [x] 5.4 Restore the sidebar footer Settings entry and move preserved Hub account, usage, referral, and logout actions into the lazy compatibility surface without affecting local Provider authority.
- [x] 5.5 Add focused main/renderer tests for setup transaction order, splash transition, fresh and existing profiles, unusable Provider recovery, direct Settings navigation, and zero ordinary Hub initialization.
- [x] 5.6 Complete K5 only when a fresh ordinary start reaches local setup, a ready local profile reaches the workspace, and neither path loads or exercises Hub behavior unless the user explicitly enters compatibility.

## 6. K6 — Complete Provider Settings and routing controls

- [x] 6.1 Implement Provider Settings list, add, edit, select, disconnect, explicit credential delete/replace, and redacted status/error flows using the local API.
- [x] 6.2 Implement built-in and custom Provider endpoint configuration, proxy handling, credentialed health probes, and secret-free probe diagnostics.
- [x] 6.3 Implement model and media-model discovery, manual model configuration, selection recovery, and key-free metadata persistence.
- [x] 6.4 Implement route-pool configuration and deterministic local selection/fallback among committed Registry connections without any Hub gateway fallback.
- [x] 6.5 Add focused API/UI tests for custom Providers, probes, proxy behavior, model/media discovery, route changes, stale revisions, credential no-op updates, explicit deletion, and redaction.
- [x] 6.6 Complete K6 only when all Provider/API-key settings behavior is available through the single Registry, produces key-free public state, and cannot select Hub as an implicit authority.

## 7. K7 — Migrate every existing model consumer

- [x] 7.1 Produce and check in a bounded consumer inventory covering chat, graph, subagents, write flows, schedules, messaging integrations, image generation, speech/transcription, media, and every runtime launch or Provider helper that currently reads settings or Hub credentials.
- [x] 7.2 Move chat, graph, subagent, write, schedule, and messaging model execution to committed Registry resolution and the existing Go runtime boundary.
- [x] 7.3 Move image, speech/transcription, and other media-model consumers to the same Registry and Secret Store authority, including custom endpoints, proxies, and selected media models.
- [x] 7.4 Remove direct credential reads, copied Provider configs, and long-lived secret caches from all inventoried consumers; enforce re-resolution or fail-closed behavior after revision/generation change.
- [x] 7.5 Add focused per-consumer tests for correct Provider selection, secret-free state/events/errors, update and revocation propagation, unavailable credentials, and absence of Hub fallback.
- [x] 7.6 Complete K7 only when the checked inventory has no unresolved consumer and every ordinary model call obtains authority from the Registry without a settings, Hub, or private side channel.

## 8. K8 — OAuth, subscription, CLI, quota, tray, MCP OAuth, and extension accounts

- [x] 8.1 Move Provider OAuth authorization, refresh, revocation, and subscription credentials into the Secret Store with Registry references and revision/generation CAS.
- [x] 8.2 Move CLI Provider selection and credential use to the local Registry while keeping CLI metadata, diagnostics, and errors key-free.
- [x] 8.3 Move quota and tray status to redacted Registry/Provider observations without Hub account refresh or persistent credential copies.
- [x] 8.4 Move MCP OAuth and extension Provider accounts into the same protected Secret Store and fenced mutation/recovery lifecycle.
- [x] 8.5 Add focused tests for OAuth refresh versus user update, subscription changes, CLI execution, quota/tray refresh, MCP OAuth recovery, extension-account revocation, redaction, and stale-authority failure.
- [x] 8.6 Complete K8 only when every listed account/token feature uses the one local authority and a stale background refresh cannot overwrite a newer user credential.

## 9. K9 — Import, export, cross-device migration, and credential recovery

- [x] 9.1 Define and implement the strict canonical `analytix.provider-portable-manifest/v1` projection containing only bounded key-free Provider/account descriptors, normalized endpoint/proxy and model/media metadata, manifest-local ordered route correlation, public OAuth binding/account-observation metadata, and per-entry `reentry_required` or `protected_recovery_available` intent; sort Provider correlations by stable `(kind, normalized endpoint)` identity, use account `(owner, portable owner descriptor, purpose)` identity, and exclude credential bytes, recovery codes, private keys, `credentialRef`/account secret references, Registry state, source authority IDs, device identity, trust/permissions, store paths/payloads, logs, and user data.
- [x] 9.2 Implement ordinary import with complete canonical/bounds/security preflight before Registry mutation; allocate and destination-mint every Provider/account/scope/fence/reference identity before one logical commit, map only manifest-local ordered routes to destination Provider IDs without changing Registry selection, create uncredentialed unselected disconnected/non-executable entries, preserve supported public OAuth/observation metadata, reject unsupported account proxy/authorization-state/transport classes, and require normal protected re-entry before selection, route admission, or execution.
- [x] 9.3 Implement an explicit protected cross-device credential-recovery flow that authenticates the action, re-encrypts under the destination Secret Store, and never treats source-device keys, references, trust, or permissions as destination authority; keep this separate from the Closure A ordinary manifest.
- [x] 9.4 Implement migration rollback, destination conflict handling, interrupted transfer recovery, source preservation, and explicit post-verification cleanup without credential loss; duplicate/canonical collisions, ambiguous route mappings, non-terminal Legacy Migration Recoveries, capacity fences, or Store commit failures are strict all-or-none failures with no Secret Store recovery/read, no overwrite/merge/keep-both/source-ID reuse/partial success, and no physical-filesystem-atomicity claim. The Store port depends on journal/exact-readback/prior-byte-rollback evidence across restart.
- [x] 9.5 Add archive/content audits and focused tests for strict v1 round trips, malicious/legacy/path-bearing/symlink-or-archive-traversal/duplicate-key/unknown-security-critical/secret-or-trust-canary/noncanonical manifests, excluded paths and fields, destination minting and re-entry state, and ordinary Closure A conflict/CAS behavior. Final K9 evidence must also cover destination re-encryption, protected request/bundle/receipt and crypto, interruption/rollback, and secret/trust non-transfer; these obligations remain mandatory for 9.5/9.6 and are not deferred.
- [x] 9.6 Complete K9 only when ordinary artifacts are provably key-free and cross-device recovery requires an explicit protected flow that preserves credentials without transferring trust or authority.

## 10. K10 — Hub-zero proof and real Electron/package acceptance

- [x] 10.1 Add deterministic source/build-graph checks that fail on any ordinary-startup Hub static import, service construction, refresh timer, token read path, account/model request path, or gateway fallback while allowing only the explicit lazy compatibility boundary.
- [x] 10.2 Add runtime observations that distinguish zero ordinary Hub module load, instance, timer, token read, network request, and fallback from an explicitly invoked deprecated compatibility action.
- [x] 10.3 Run real Electron development acceptance with a fresh isolated profile and data directory, entering an authorized developer-owned credential only through normal protected onboarding or Provider Settings and verifying Registry/recovery/key-free/consumer/Hub-zero behavior.
- [x] 10.4 Build and run a fresh local-nonpublishable package with bootstrap disabled and a clean profile; have the package user enter their own credential through normal protected UI and repeat the product-seam observations without copied developer state.
- [x] 10.5 Retain only redacted or synthetic evidence, including explicit checks that source, package, settings, renderer/public IPC, logs, telemetry, screenshots, exports, and evidence contain no real credentials, raw Provider bodies, user data, or copied Secret Store material.
- [x] 10.6 Run the focused and cross-layer verification required by the changed security, migration, Electron, runtime, import/export, and packaging surfaces, preserving literal pass/fail/blocked/not_executed outcomes.
- [x] 10.7 Complete K10 only when ordinary Hub activity is observed as zero in both real Electron seams, the fresh package uses package-user credential entry through the single authority, every K1–K9 completion criterion remains green, and no publishable or Formal RC claim is inferred from this local-nonpublishable acceptance.
