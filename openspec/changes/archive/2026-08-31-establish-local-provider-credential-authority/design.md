## Context

Analytix currently has accepted requirements that make Hub authorization, Hub model discovery, and Hub account state part of ordinary desktop startup. The target state instead requires local Provider configuration and protected local credentials to be the only ordinary provider authority. The change crosses persistence, security, Electron IPC, runtime composition, migration, UI, and packaging, so it needs one staged design before implementation.

The existing product architecture remains authoritative: the renderer calls `window.analytix`, preload exposes a narrow API, main owns desktop services and runtime composition, and `packages/runtime-go` remains the only production Agent runtime. Kun tag `v0.3.6` at commit `993f3152beec0070eb5c1773d59ab36889a13203` is a bounded behavioral reference for Provider and credential lifecycle coverage; Analytix owns the resulting contracts and implementation, and no Kun runtime or source is adopted.

## Goals / Non-Goals

**Goals:**

- Establish one durable Analytix-local Provider Registry and one protected Secret Store as the credential authority for every Provider and credential-bearing consumer.
- Keep settings, renderer state, public IPC, logs, telemetry, imports, exports, and evidence free of credential bytes.
- Make registry mutations, legacy migration, crash recovery, credential refresh, deletion, and rollback deterministic and loss-resistant under concurrent or interrupted execution.
- Retire Hub from ordinary startup while preserving an explicit lazy compatibility and migration path.
- Cover local Provider setup, custom Providers, probes, proxies, model discovery, route pools, OAuth, subscriptions, media models, CLI, quota, tray, MCP OAuth, extension accounts, import/export, cross-device migration, and recovery.
- Prove the target at real Electron development and fresh local-nonpublishable package seams using isolated profiles and protected credential entry.

**Non-Goals:**

- Introducing another runtime, provider-execution loop, settings authority, credential store, or renderer-accessible secret API.
- Deleting Hub source or legacy data before the compatibility and migration contract has completed.
- Copying or embedding an upstream runtime, source tree, credential value, pre-authenticated profile, or developer key.
- Claiming publishable, release, Product, Package, or Formal RC acceptance from this planning change.

## Decisions

### 1. Main owns one local Registry; Go remains the only production runtime

Electron main will compose a single data-directory-scoped Secret Store, Provider Registry, and local Provider API. Preload will expose only typed, redacted operations and renderer will consume only key-free connection snapshots. Main will resolve an opaque `credentialRef` just in time for the existing runtime/provider launch or update boundary; the Go runtime continues to own Agent execution and provider egress.

This separates configuration authority from execution authority without creating a second runtime. A renderer-owned registry, a parallel development registry, and direct settings-to-runtime credential reads are rejected because each would create competing authority or expose secrets to broader surfaces.

### 2. Registry metadata and encrypted credential bytes are separate

The Registry will persist versioned connection metadata under the product data directory: Provider identity and kind, endpoints, proxy and model metadata, selected routes, status, opaque `credentialRef`, revision, generation, incarnation, tombstone, and transaction state. It will never persist credential bytes, OAuth tokens, subscription tokens, or a reversible secret in settings or metadata.

The Secret Store will persist authenticated encrypted envelopes separately. The initial envelope uses AES-256-GCM with a random nonce, authentication tag, version marker, and purpose-bound associated data. The master key uses the operating-system credential facility where supported, including macOS Keychain and Windows current-user protection. Fallback master-key initialization uses restrictive permissions and exclusive creation, never overwrites or replaces an existing key, and requires concurrent losers to re-read the winner. Atomic replacement is limited to encrypted credential envelopes, Registry metadata, and other non-key transaction files. An existing-key permission or readback mismatch fails closed without replacing the key or overwriting ciphertext or metadata.

Settings store only the selected Provider/model and other key-free preferences. Public types use `credentialRef` solely as an opaque capability reference; no public operation resolves it to bytes. A missing, redacted, masked, or empty credential input means “leave the committed credential unchanged.” Credential removal requires the Registry's explicit delete or disconnect operation.

### 3. Every credential mutation uses fence, prepare, commit, and CAS

The Registry will serialize mutations by data directory and require an expected revision. A mutation records a fence containing the Registry incarnation, Provider generation, current credential reference, expected revision, and operation identity. Prepare durably writes the encrypted candidate and transaction intent before metadata can reference it. Commit uses revision compare-and-set to publish the new metadata/reference and selection state. Only after the committed Registry state is durable may cleanup tombstone the superseded reference.

Recovery is idempotent: it reconciles prepared intents, committed revisions, ciphertext presence, tombstones, and generation/incarnation before choosing roll-forward or rollback. It preserves the last committed usable credential, never guesses across an ambiguous fence, and never deletes an unreferenced secret until the recovery scan proves it belongs to a completed supersession or explicit delete. OAuth refresh and other background credential rotation use the same compare-and-set rule so a stale refresh cannot replace a newer credential.

### 4. Migration is source-preserving until a new authority commit succeeds

Legacy plaintext Provider settings and legacy Provider metadata are local compatibility inputs. A migration reads them into an isolated transaction, validates the target Provider, writes the encrypted credential, commits Registry metadata, verifies readback, and only then removes plaintext from ordinary settings. Once cleaned, the exact verified key-free ordinary representation remains unchanged through rollback and recovery. A protected recovery record retains the credential-bearing prior representation without leaving plaintext in settings. Invalid, incomplete, or conflicting input fails closed and remains recoverable.

Legacy Hub state is different: ordinary startup must not import Hub modules or read Hub tokens. Only an explicit user-invoked compatibility or migration flow may lazy-load the Hub adapter, inspect authorized legacy state, and convert a supported Provider into the local Registry through the same transaction. It never enables a gateway fallback.

Rollback operates from the last committed Registry snapshot plus protected recovery records. Before commit, rollback removes only the prepared candidate and leaves the legacy source authoritative. After commit, rollback authenticates the exact cleaned ordinary source, recovery payload, Registry fences, and authorized Secret Store readback; it then commits either the exact still-valid prior protected Registry winner or a key-free recovery-retained terminal with no active migrated Provider. The encrypted migration recovery remains protected and non-resolvable by ordinary consumers while migrated-winner cleanup resumes idempotently from a durable cleanup-pending state. Reusing that recovery requires a new prepare/fence/CAS/commit/readback remigration transaction, and deleting it requires a separate explicit protected delete with its own terminal marker.

### 5. One local Provider API carries all features and consumers

The main-owned API will provide list/get, connect, update, select, disconnect/delete, credential replace, model discovery, health probe, proxy, custom endpoint, model and media-model selection, route-pool, OAuth, subscription, and recovery operations with redacted responses. The same Registry/Secret Store composition is used in development and packaged builds; environment or `.env` credentials do not form another product configuration path.

All model consumers must resolve Provider state through this authority: chat, graph, subagents, write tools, schedules, messaging integrations, media generation and transcription, quota/tray, CLI, MCP OAuth, and extension accounts. Consumers may cache a value only for the bounded execution that requested it and must re-resolve or fail closed after revision, generation, credential, or authority changes.

Import/export and cross-device migration carry only key-free Provider metadata and an explicit manifest of credentials that must be re-entered or recovered through a protected transfer flow. Ordinary archives exclude credential bytes, master keys, credential references, OAuth and subscription tokens, Hub tokens, device identity, trust and permission state, and Secret Store or OAuth storage paths.

### 6. OAuth bindings and native callbacks are Main-owned

Provider OAuth bindings are versioned key-free Provider Registry metadata and change only through the Provider account-management operation under the current Provider fence. MCP OAuth bindings are owned by the exact Main-normalized MCP server configuration and its current fingerprint; extension bindings are owned by the verified installed manifest, server configuration, and source fingerprint. A binding or owner-generation change invalidates pending authorization, refresh, revocation, and stale callbacks. Bindings contain only the issuer, normalized authorization/token/optional revocation endpoints, public client identifier, ordered scopes, and code-owned redirect version; confidential-client credentials are unsupported.

OAuth authorization is Main-only. Main commits the protected pending record before opening the external user agent and derives `com.analytix.desktop:/oauth/callback/<binding-key>` from the exact owner/provider/account/channel/issuer binding. Renderer and preload receive only a bounded authorization identifier, expiry, and redacted phase; they never receive the authorization URL, state, nonce, PKCE verifier, callback code, token, or credential reference. macOS `open-url` and Windows/Linux second-instance argv route only the exact scheme/path. Callback handling selects one protected pending record by state plus binding-key, rejects ambiguity, replay, lateness, wrong profile/window/binding, mixed success/error, and issuer mix-up, and never accepts callback-supplied binding or endpoint authority.

### 7. Hub is cold on every ordinary startup path

The ordinary main and renderer startup graph will have no static Hub import. It will not instantiate a Hub service, schedule a refresh timer, read a desktop or gateway token, send a Hub request, use Hub account readiness, or fall back to a Hub gateway Provider. The sidebar footer opens Settings directly and does not refresh account, quota, or referral state.

Deprecated Hub account and migration surfaces remain behind an explicit lazy boundary. Entering that surface may load its adapter and operate under its own user-visible lifecycle, but leaving or never entering it has no Hub process, timer, credential read, request, or Provider effect.

### 8. Acceptance uses isolated, observable product seams

Development acceptance uses a new isolated profile and data directory. A developer-owned credential is entered only through normal onboarding or Provider Settings and is stored by the protected authority. Fresh-package acceptance uses a newly built local-nonpublishable package, a clean profile, bootstrap disabled, and credentials entered by the package user through the same product flow; no developer credential or pre-authenticated profile is copied in.

Acceptance observes Registry transaction and recovery behavior, key-free settings/IPC/renderer state, consumer routing, import/export exclusions, and the absence of ordinary Hub imports, instances, timers, token reads, requests, and gateway fallback. Synthetic credential values are used in automated evidence; real credential bytes and raw Provider bodies are excluded from artifacts and evidence.

### 9. K9 ordinary portability is a strict, key-free destination projection

Closure A defines one strict canonical ordinary manifest identified by
`analytix.provider-portable-manifest/v1`. Export is an app-layer projection of
the committed local Provider authority, not a serialization of Registry state.
It may contain only bounded, key-free Provider and account descriptors: the
normalized endpoint and proxy (without userinfo or secret-bearing query
parameters), model and media-model metadata, and routes used only as
manifest-local correlation. Routes carry no source selection authority. Each
entry declares exactly `reentry_required` or
`protected_recovery_available`; the latter is only a statement that a separate
protected recovery protocol may be available, not a portable credential or
trust grant.
Closure A export emits only `reentry_required` until the separate Closure B
protocol has an exact Owner-approved transferable class; accepting the second
intent is schema compatibility, not evidence that protected recovery exists.

Provider correlation is assigned only after a deterministic sort by the stable
v1 Provider identity `(kind, normalized endpoint)`. Account identity is the
stable tuple `(owner, portable owner descriptor, purpose)`. Proxy, model/media
lists, selected model/media, OAuth/observation metadata, and endpoint changes
are mutable descriptor differences; they do not make a duplicate identity
safe. An identity collision is therefore rejected even when those mutable
fields differ.

The correlation values are deterministic: Providers use exactly
`provider-<zero-based sorted index>` and accounts use exactly
`account-<zero-based sorted index>`. Ordered routes may reference only those
Provider correlation values.

Portable metadata rejects U+2028 and U+2029 so Go and TypeScript canonical JSON
bytes agree; ordinary `<`, `>`, and `&` metadata remains representable.

An exported ordered route is translated from the source route target to the
target Provider's manifest correlation. A route targeting a tombstone, private
account, unexported Provider, or ambiguous identity fails export; empty/default
routes remain empty and route order is never sorted. Import allocates all
destination identities first, maps those correlations to destination-minted
Provider IDs, and may persist only translated `provider:<destination-id>`
routes. Registry selection remains unchanged, and selection or route admission
requires every route target to have a normal K2-committed credential.

Public Provider OAuth binding eligibility and key-free account observation
metadata round-trip as explicit descriptor fields. OAuth authorization-state
records, transport accounts, and unapproved account classes fail closed rather
than being silently dropped. MCP access-token and extension provider-account
descriptors are metadata-only; a nonempty account proxy is rejected in v1.
For an accepted private account descriptor, the destination mints the private
scope Provider, account, and channel components and carries the source owner
descriptor only as non-authoritative manifest metadata.

The ordinary manifest excludes credential bytes, recovery codes, private keys,
opaque `credentialRef` or account secret references, OAuth/subscription/Hub
tokens, Registry revision/generation/incarnation/tombstones/transactions,
source Provider or account IDs as authority, device or install identity,
trust/permission state, store paths or payloads, logs, and user data. The
destination mints every Provider/account identity and every Registry fence and
reference. Imported entries are uncredentialed, unselected, disconnected and
non-executable, and are explicitly `reentry_required`; normal K2 protected
credential commit and authorized readback are required before select, route
admission, or execution can succeed.

Import performs strict parse, bounds, canonicality, and security validation for
the complete manifest before any Registry mutation, then uses a logical
preflight -> staging -> journal -> verify/rollback shape. Duplicate entries,
canonical Provider/account collisions, and ambiguous mappings fail as one
all-or-none operation: import never overwrites, merges, keeps both, reuses a
source ID, or reports partial success. This is a logical Registry guarantee and
does not claim physical filesystem atomicity. Malformed, over-bound, legacy,
path-bearing, symlink/archive-traversal, duplicate-key,
unknown-security-critical, secret/trust-canary, or noncanonical input fails
closed with key-free public results before mutation.

The Store port's commit contract is the dependency boundary for the durable
shape: the production filesystem Provider Registry implementation journals the
candidate, verifies exact readback, and completes the prior-byte rollback
evidence protocol needed across failure and restart. Closure A claims logical Registry all-or-none behavior and
restart/readback evidence, not physical file movement or deletion atomicity.
Ordinary import rejects every non-terminal Legacy Migration Recovery before
changing the Registry revision; only the already-terminal records permitted by
ordinary mutations may coexist, and no Secret Store recovery or read occurs.

Kun v0.3.6 remains only an inspect/preflight -> staging -> journal ->
verify/rollback shape reference. Its installation, passphrase, or signature
semantics are not Analytix trust.

## Protected cross-device credential recovery (Closure B)

Closure B is a separate, explicit protected offline protocol that starts only
after Closure A ordinary import has completed. Ordinary import produces a
Main-owned, key-free `PortableImportReceipt` containing the exact canonical
manifest bytes and the bounded correlation-to-destination-minted entry result,
including its Registry fences. The destination Manager must strict-parse and
re-hash those bytes and cross-check every descriptor, correlation, entry, and
fence against the current imported uncredentialed Provider/account metadata
before creating any pending state; a caller-supplied digest paired with an
arbitrary entry list is not sufficient. The pending session is Main-owned and
is where the current BrowserWindow, mainFrame, profile, data-directory, owner
bindings, and native confirmation state are kept. None of those local
authority values is exported in a request, bundle, or receipt. The protocol is
not a Hub migration, a transport channel, or a trust transfer.

The protocol has three strict canonical UTF-8 JSON records, each with an exact
schema identifier and protocol version `1`:

- `analytix.provider-protected-recovery-request/v1` is created by the
  destination after ordinary import. It contains the canonical manifest
  digest, a random operation/session identifier and nonce, an expiry, the
  destination ephemeral X25519 public key, a short verification fingerprint,
  and the complete ordered manifest-correlation to destination-minted entry
  ID and per-entry Registry-fence map. The request contains no profile,
  data-directory, window/frame, device identity, source ID, source fence,
  credential, or private key. The request does not serialize `requestDigest`:
  the parser computes it over the canonical request payload with the derived
  fingerprint field omitted and verifies the fingerprint. The fingerprint is
  the lowercase hex encoding of the first eight bytes of SHA-256 over that
  request digest; the computed digest is carried by the bundle and receipt.
- `analytix.provider-protected-recovery-bundle/v1` is created by the source
  from one matching request. It contains only the request and manifest
  digests, operation/session transcript fields, the ordered
  source-correlation-to-destination-entry map, the allowlisted purposes, a
  source ephemeral X25519 public key, and one bounded AES-256-GCM envelope
  over the canonical ordered credential payload. It contains no source
  Provider/account IDs, credential references, source Registry/K1 snapshot,
  profile/data-directory/device binding, selection, trust, permissions, or
  plaintext.
- `analytix.provider-protected-recovery-receipt/v1` is created by the
  destination only after a successful destination Registry-lock batch,
  destination K1 re-encryption, CAS, and exact readback. It contains only the
  request, manifest, and bundle transcript digests, operation/session fields,
  bounded commit/readback status, expiry, and the exact correlation to
  destination-minted ID/status map plus an exact 32-byte
  transcript-authentication tag. The tag is derived from the transfer key
  with a domain-separated receipt-authentication subkey and binds the
  canonical receipt transcript and complete recovery AAD. It is key-free and
  contains no path,
  profile/window binding, bundle ciphertext, credential, private key, source
  authority, or source Registry snapshot.

All three records use exact canonical JSON with no duplicate keys, unknown
fields, trailing data, legacy schema, noncanonical bytes, over-bounds,
path-bearing or archive/symlink traversal material, or security-sensitive
canaries. Every digest is SHA-256 over the exact canonical UTF-8 bytes of its
record or named sub-record. The request, bundle, and receipt are each bounded
before any durable mutation; a malformed or mismatched record is rejected
without Registry or Secret Store mutation. Local pending/session records may
retain their Main context and protected state, but those values are not part
of the external records.

The cryptographic binding is fixed. The source derives an X25519 shared secret
from the destination public key in the request and a source ephemeral private
key. HKDF-SHA-256 uses salt equal to the request digest and info exactly
`analytix.provider-protected-recovery/hkdf/v1`, producing a 32-byte AES-256-GCM
key. The destination ephemeral private key is protected only under
destination K1 for bounded local restart recovery and is never exported. The
bundle uses a 12-byte nonce and authenticates the canonical ordered
credential payload.

The AAD is exactly the four-byte-big-endian-length-prefixed UTF-8
concatenation, in this order, of: (1) the protocol/schema version, encoded as
the exact schema identifier plus protocol version; (2) request digest; (3)
manifest digest; (4) destination ephemeral public key; (5) source ephemeral
public key; (6) operation/session identifier; (7) operation/session nonce;
(8) the ordered source-correlation-to-destination-entry mapping; (9) the
ordered credential purposes; and (10) expiry. Mapping and purposes use their
canonical order and byte encoding. Destination fences are transitively bound
by the canonical request digest and are not substituted with profile, path, or
source Registry snapshots in AAD.

There are exactly two fresh native Main confirmations in the ordinary path:
one destination confirmation during request/session creation and one source
confirmation before bundle creation. Both confirmations cover the same short
request fingerprint, request/manifest digest, exact local operation,
profile/window/frame, item set, and expiry. Destination apply consumes its
still-current unconsumed local confirmation/session; after restart or any
currentness drift, apply/resume requires a new destination confirmation and
never publishes automatically. Receipt finalization is exact receipt/session
matching and cleanup and does not add a third or fourth prompt. Current MCP
and extension binding fingerprints are resolved by Main-owned inventories; a
missing or incompatible owner binding fails the whole protected batch before
candidate commit.

The Manager, rather than the caller, owns protected-session authority. The
destination Manager's prepare operation accepts only the exact canonical
`PortableImportReceipt`, the authenticated Main local binding, and the
complete current destination MCP/extension owner-binding inventory. It
strictly parses and verifies the receipt against current Registry state, then
generates the operation/session identifier and nonce, bounded expiry, and
ephemeral X25519 key pair, protects the private key under destination K1, and
durably stores an **unconfirmed** pending session. It returns request bytes and
only the computed digest, short fingerprint, item-set digest, and expiry needed
for the native confirmation. A separate private destination-confirm operation
records the exact fresh confirmation and current destination inventory in that
Manager-owned session; only a confirmed session may write the request file.
Cancel or failed confirmation rolls back the unconfirmed pending state. Apply
loads the exact confirmed pending session by request/operation digest from the
Manager's Registry/K1 records, re-resolves the current destination inventory,
and accepts no caller-supplied session, key, fence, or confirmation-consumed
state. The source Manager derives current source owner, purpose, and fence from
its own canonical manifest and Registry; its only external authority inputs are
request bytes, an exact current Main local-action confirmation, and the
complete Main-owned source MCP/extension binding inventory. Finalize loads its
own durable source transfer session by the receipt transcript. No
caller-selected purpose, source Provider/account ID, source credential
reference, or source session is authoritative.

Main owns the native open/save dialogs and validates the actual invoke sender,
current BrowserWindow/mainFrame, profile, and data-directory before each
operation. The four offline operations are: (1) destination Main calls its
Main-owned `getPortableImportReceipt()` dependency, privately prepares an
unconfirmed request, shows
the one destination confirmation, privately records that confirmation, then
saves and writes the request; (2) source Main opens and reads the request,
confirms, privately creates the bundle from its own current owner inventory,
then saves and writes the bundle; (3) destination Main opens and reads the
bundle, privately applies it after re-resolving the current destination owner
inventory, re-encrypts under destination K1 in one Registry-lock batch with
CAS/readback, then saves and writes a key-free receipt; (4) source Main opens
and reads the matching receipt and privately finalizes the protected session
and exact app-owned temporary cleanup. The normal Main sequence therefore
has five private calls—prepare request, confirm destination, create source
bundle, apply destination bundle, and finalize source receipt—exactly two
native confirmations, three native opens, three native saves, and three
writes. Renderer and preload supply no path, file handle, purpose, binding
authority, or secret/bundle bytes; they receive only operation-specific
write-only invocation and bounded redacted status. Generic runtime requests
cannot reach these private routes, and Go uses one operation-specific private
K1 consumer for source reads plus private operation-specific consumers for
destination pending-key protection and batch readback.

The only transferable credential purposes are `provider-api-key`,
`provider-oauth-token-bundle` (including subscription data only inside that
bundle), `mcp-oauth-access-token`, and
`extension-provider-account-token`. Hub credentials, OAuth authorization-state
records, transport credentials, all other credential classes, trust or
permission state, device/install identity, source keys, source identifiers,
and opaque references are rejected. Destination K1 is the only credential
winner; selection and route authority are not transferred automatically.

Destination apply is logically all-or-none: every entry, purpose, binding,
fence, and map is preflighted before the single Registry-lock batch. A failed
CAS, commit, readback, expiry, owner/fence drift, missing or incompatible
source/destination MCP/extension binding, or partial map leaves the prior
committed destination Provider/account/credential state and the source Provider/account/credential
state unchanged, with no partial winner. Bundle replay is rejected after the
pending session is consumed; the verified receipt remains transcript-bound for
source finalization and audit. A pre-terminal interruption or restart
preserves the prior committed destination Provider/account/credential
authority while retaining recoverable encrypted pending/session/candidate
records; protected journal/Registry metadata may advance. Pending candidates
are never visible, selectable, or executable. Resuming requires fresh
destination currentness and confirmation; otherwise rollback removes only the
exact candidates and private-key artifacts while retaining the imported
uncredentialed entries and their fences. This is a logical
persistence guarantee supported by the production store's journal/readback/
rollback contract, not a claim of physical filesystem atomicity. Cleanup may
remove only exact app-owned protected
scratch/candidate artifacts after a verified receipt; an external or
user-copied file is never claimed erased and best-effort zeroization is not a
durability guarantee. Source finalization changes only its protected transfer
session/scratch records, never the source Provider, account, credential,
selection, or source authority.

The protected journal has explicit `pending_key_candidate` (the pre-K1
journal), `pending`/unconfirmed, `confirmed`, `candidates_durable`, `verification_pending`, `applied`/receipt-ready,
`source_bundle`, `cleanup_pending`, `rollback_pending`, `finalized`, and
`rolled_back` phases. The destination-key reference and prior key-free fences
are journaled in `pending_key_candidate` before K1 commit; a failed K1
commit/readback or pending-publication transition leaves only an exact
retryable cleanup record and never publishes a request. Before the Provider
batch, every K1 candidate is
committed and exact-byte read back through the operation-specific recovery
consumer. The one Registry-lock batch records `verification_pending` together
with the candidate references, purposes, transcript-bound digests, and exact
prior destination metadata/fences needed for retry or rollback. Before any
ordinary Snapshot, selection, account resolution, or execution returns,
`recoverAll` must reload and verify the successor, every fence and owner
mapping, the receipt transcript, and the exact K1 candidate bytes; a mismatch
restores the prior logical Registry state. A failed rollback or cleanup keeps
the phase and exact retry metadata durable and fails closed. After verified
terminal readback, the ephemeral destination key is cleaned through the same
retryable tombstone/delete protocol. Pre-terminal restart may retain encrypted
pending/session/candidate records, but prior committed Provider/account/
credential authority remains unchanged and no pending candidate is visible,
selectable, or executable. Source K1 reads and source-session publication use
one locked currentness view or an exact second fence/owner check before the
bundle is returned.

## Risks / Trade-offs

- [Interrupted migration could strand a credential] → Keep legacy source or a protected recovery record until encrypted write, Registry CAS commit, and readback all succeed; recover idempotently from the durable transaction state.
- [Concurrent UI, OAuth, or consumer updates could overwrite a newer secret] → Require revision CAS plus generation/incarnation fences for every mutation and refresh.
- [Fallback master-key handling could weaken storage] → Prefer the OS credential facility, enforce exclusive restrictive fallback creation, and fail closed on permission or readback mismatch.
- [A redacted form could erase a credential] → Treat empty/redacted input as no-op and require an explicit delete/disconnect operation.
- [A hidden Hub import could restore startup traffic] → Add build-graph and real Electron observations for imports, service creation, timers, token reads, requests, and fallback selection.
- [Cross-device transfer could leak trust or secrets] → Export key-free metadata only by default and require an explicit protected credential-recovery flow on the destination.
- [A package test could accidentally use developer state] → Require a fresh package, clean isolated profile, bootstrap disabled, and package-user credential entry.

## Migration Plan

1. Build and verify the Secret Store and master-key lifecycle.
2. Build the Registry, local API, revision CAS, fence, prepare/commit, deletion, and recovery state machine.
3. Make settings and public DTOs key-free; migrate and recover legacy plaintext Provider state transactionally.
4. Compose the shared development/packaged data directory, Manager, Registry, and runtime resolution boundary.
5. Move first setup and startup gating to local Provider readiness; restore the sidebar Settings entry and cold-retire ordinary Hub UI behavior.
6. Complete Provider Settings, custom endpoints, probes, proxies, models, media models, and route pools.
7. Move every existing model consumer to Registry resolution.
8. Move OAuth, subscriptions, CLI, quota, tray, MCP OAuth, and extension accounts to the same authority.
9. Complete key-free import/export, cross-device migration, credential recovery, and secret/trust exclusions.
10. Prove zero ordinary Hub activity and run real Electron development plus fresh local-nonpublishable package acceptance.

Rollback at each stage restores the last committed Registry revision and preserved recovery state; no stage may delete the prior usable credential before its successor is committed and verified.

## Open Questions

None. Implementation may choose concrete module names and storage filenames within these contracts, but it must not introduce another authority or weaken the migration, security, or acceptance requirements.
