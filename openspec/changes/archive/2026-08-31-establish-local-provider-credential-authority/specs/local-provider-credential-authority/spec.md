## ADDED Requirements

### Requirement: Analytix has one local Provider and credential authority
Analytix SHALL use one data-directory-scoped local Provider Registry and one protected Secret Store as the durable authority for Provider configuration and credential bytes. The Registry SHALL integrate through the existing Electron main/preload/renderer boundary, and `packages/runtime-go` SHALL remain the only production Agent runtime and provider-execution owner.

#### Scenario: Ordinary Provider state is composed
- **WHEN** Electron main composes Provider services for a profile
- **THEN** it creates one Registry and Secret Store for that profile and exposes only typed key-free operations through preload
- **AND** it does not create a renderer-owned, development-only, Hub-owned, or second runtime credential authority

#### Scenario: A production model request executes
- **WHEN** an Analytix consumer requests a model through a selected local Provider
- **THEN** main resolves the committed Provider and credential for the existing Go runtime execution boundary
- **AND** no TypeScript or Hub Agent runtime is selected or introduced

### Requirement: Secret bytes are protected and public surfaces are key-free
The Secret Store SHALL keep encrypted credential bytes separate from key-free Registry metadata and settings. Registry and public objects MAY carry an opaque `credentialRef`, but renderer state, settings, public IPC, logs, telemetry, imports, exports, diagnostics, and evidence SHALL NOT contain credential bytes or a reversible secret.

#### Scenario: A credential is saved
- **WHEN** a user connects or updates a Provider with a credential
- **THEN** authenticated encrypted bytes are committed to the Secret Store and Registry metadata stores only an opaque reference
- **AND** settings store only key-free Provider and model preferences

#### Scenario: A public connection snapshot is returned
- **WHEN** renderer, CLI metadata output, or another public consumer lists Provider connections
- **THEN** the response contains redacted capability and status metadata without API keys, OAuth tokens, subscription tokens, master keys, or raw Provider bodies

#### Scenario: A form submits an empty or redacted key
- **WHEN** an update contains an omitted, empty, masked, or redacted credential field without an explicit delete operation
- **THEN** the Registry preserves the currently committed credential unchanged

#### Scenario: Credential deletion is requested
- **WHEN** a user explicitly invokes Registry delete or disconnect for a Provider credential
- **THEN** the Registry records and commits the deletion transaction before the Secret Store tombstones eligible bytes

#### Scenario: The master key or envelope is unreadable
- **WHEN** the Secret Store cannot authenticate or decrypt a committed credential
- **THEN** the affected credential operation fails closed without overwriting metadata, ciphertext, or another usable credential

### Requirement: Registry mutations are crash-safe and concurrency-safe
Every credential-bearing Registry mutation and background credential refresh SHALL use revision compare-and-set plus generation/incarnation fencing, a durable prepare record, an atomic commit decision, and idempotent recovery. The Registry SHALL preserve the last committed usable credential until a successor or explicit deletion is committed and verified.

#### Scenario: Two writers update the same Provider
- **WHEN** two operations prepare from the same Registry revision
- **THEN** at most one commit succeeds and the stale writer receives a conflict without replacing or deleting the winner's credential

#### Scenario: The process stops after credential prepare
- **WHEN** encrypted candidate bytes and a prepare record are durable but Registry metadata has not committed the new reference
- **THEN** restart recovery rolls forward only when the fence and commit evidence match, otherwise rolls back the candidate and preserves the last committed reference

#### Scenario: The process stops after metadata commit
- **WHEN** Registry metadata commits a new reference before superseded-secret cleanup finishes
- **THEN** restart recovery keeps the committed reference usable and resumes cleanup without restoring the superseded reference as current

#### Scenario: OAuth refresh races with a user update
- **WHEN** a background refresh uses a stale credential generation after the user has committed a newer credential
- **THEN** its compare-and-set fails and the newer credential remains authoritative

### Requirement: Legacy inputs migrate without credential loss
Analytix SHALL migrate legacy plaintext Provider settings and legacy Provider metadata through the same protected Registry transaction, and SHALL expose legacy Hub state only through an explicit lazy compatibility or migration flow. Migration, recovery, rollback, and cleanup SHALL preserve a recoverable credential until the successor is committed and verified, while a cleaned ordinary source remains exact and key-free.

#### Scenario: Legacy plaintext Provider settings are detected
- **WHEN** local Registry bootstrap finds a supported plaintext Provider credential in legacy settings
- **THEN** it encrypts, commits, and verifies the credential before removing plaintext from ordinary settings
- **AND** it records protected rollback information without leaving plaintext in key-free product surfaces

#### Scenario: Legacy migration is interrupted or invalid
- **WHEN** validation, encrypted write, compare-and-set, commit, or verification fails
- **THEN** the migration fails closed and preserves the prior recoverable source and last committed Registry state

#### Scenario: A user explicitly migrates legacy Hub state
- **WHEN** the user opens the deprecated compatibility or migration surface and authorizes a supported conversion
- **THEN** Analytix lazy-loads the adapter and commits the resulting local Provider through the Registry transaction
- **AND** the conversion does not enable Hub gateway fallback

#### Scenario: A committed migration is rolled back
- **WHEN** an authorized rollback selects a prior committed Registry snapshot or protected recovery record
- **THEN** Analytix keeps the exact verified cleaned ordinary source unchanged and commits either the exact still-valid prior protected Registry winner or a key-free recovery-retained terminal with no active migrated Provider
- **AND** it verifies the terminal Registry and retained recovery before idempotently deleting only the inactive migrated winner and scratch material
- **AND** the encrypted migration recovery remains protected from ordinary consumers until a separate explicit remigration transaction or explicit protected delete completes

#### Scenario: A retained migration recovery is reused or deleted
- **WHEN** an authorized user explicitly remigrates or deletes a rollback-retained recovery
- **THEN** remigration uses a fresh protected prepare, fence, compare-and-set commit, and authorized readback without changing the cleaned ordinary source
- **AND** only the separate fenced protected-delete operation removes the retained recovery and records an idempotent terminal deletion marker

### Requirement: Development and packaged builds share the product authority
Unpackaged development and packaged Analytix SHALL use the same data-directory, Manager, Registry, Secret Store, local Provider API, and credential lifecycle. Development credentials SHALL enter only through normal protected onboarding or Provider Settings in an isolated profile; packaged users SHALL enter their own credentials in a fresh profile.

#### Scenario: A developer exercises a Provider
- **WHEN** development acceptance uses a developer-owned credential
- **THEN** the credential is entered through normal onboarding or Provider Settings into an isolated data directory and is not supplied by a competing `.env`, fixture, bootstrap, or settings authority

#### Scenario: A fresh package starts
- **WHEN** a newly built local-nonpublishable package opens a clean profile
- **THEN** it has no copied developer credential or pre-authenticated profile and asks the package user to configure a Provider through the protected product flow

### Requirement: Provider features and credential consumers use the Registry
Analytix SHALL carry custom Providers, endpoint and proxy configuration, health probes, model and media-model discovery, model selection, route pools, OAuth, subscriptions, CLI use, quota and tray state, MCP OAuth, and extension accounts through the same Registry and Secret Store. Every model consumer SHALL resolve the selected committed Provider from that authority.

#### Scenario: A user configures a custom Provider
- **WHEN** the user enters an endpoint, proxy, credential, and model choices and runs a probe
- **THEN** Provider Settings uses the local API transaction and returns only redacted probe and model results

#### Scenario: A non-chat consumer requests a model
- **WHEN** graph, subagent, write, schedule, messaging, image, speech, media, CLI, MCP, quota, tray, or extension logic needs Provider or account state
- **THEN** it resolves the committed Registry entry and protected credential rather than reading settings, Hub gateway state, or its own credential file

#### Scenario: Provider authority changes
- **WHEN** a Provider is updated, revoked, disconnected, or superseded
- **THEN** bounded consumers re-resolve against the new revision or fail closed instead of using stale cached credentials

### Requirement: Import export and cross-device recovery exclude secrets and trust
Ordinary import, export, backup, and cross-device artifacts SHALL contain only key-free Provider metadata and an explicit credential-reentry or protected-recovery manifest. They SHALL exclude credential bytes, master keys, opaque credential references, OAuth and subscription tokens, Hub tokens, device identity, trust and permission state, and Secret Store or OAuth-storage payloads.

Closure A ordinary portability SHALL use one strict canonical manifest schema
identified by `analytix.provider-portable-manifest/v1`. The manifest SHALL
contain only bounded key-free Provider and account descriptors, normalized
endpoints and proxies without userinfo or secret-bearing query parameters,
model and media-model metadata, and manifest-local route correlation. Routes
SHALL carry no source selection authority. Each entry SHALL declare exactly
`reentry_required` or `protected_recovery_available`; neither intent grants a
credential or trust.
Closure A export SHALL emit only `reentry_required` until a separate Closure B
protocol defines an exact Owner-approved transferable class; accepting
`protected_recovery_available` is schema compatibility only and does not claim
that protected recovery exists.

The manifest SHALL omit recovery codes, private keys, `credentialRef`, account
secret references, Registry revision/generation/incarnation/tombstones/
transactions, source Provider or account IDs as authority, device/install
identity, trust/permissions, store paths/payloads, logs, and user data. The
destination SHALL mint every Provider/account identity and every fence/reference.
Imported entries SHALL be uncredentialed, unselected, disconnected,
non-executable, and explicitly `reentry_required`. Select, route admission, and
execution SHALL reject them until a normal K2 protected credential commit and
authorized readback succeeds.

Provider correlation SHALL be assigned after deterministic sorting by stable v1
identity `(kind, normalized endpoint)`. Account identity SHALL be the stable
tuple `(owner, portable owner descriptor, purpose)`. Mutable proxy,
model/media, selected, OAuth/observation, and endpoint metadata differences
SHALL not avoid a collision. Ordered routes SHALL be represented only as
manifest-local Provider correlations; empty/default routes SHALL remain empty,
and export SHALL reject a route to a tombstoned, private, unexported, or
ambiguous target. Import SHALL map correlations to destination-minted IDs only
after allocating all destination IDs, and any persisted translated route SHALL
use destination IDs without changing `Registry.SelectedProviderID`.
Provider correlations SHALL be exactly `provider-<zero-based sorted index>` and
account correlations SHALL be exactly `account-<zero-based sorted index>`;
ordered routes SHALL reference only those Provider correlations.
Portable metadata SHALL reject U+2028 and U+2029 to keep Go and TypeScript
canonical JSON bytes aligned; ordinary `<`, `>`, and `&` metadata remains
representable.

Public Provider OAuth binding eligibility and key-free account observation
metadata SHALL round-trip. Transient OAuth authorization-state records,
transport accounts, and unapproved account classes SHALL fail closed; supported
MCP access-token and extension provider-account descriptors SHALL remain
metadata-only. A nonempty account proxy SHALL be rejected in v1. The source
private-account owner descriptor SHALL never become destination authority:
destination scope Provider, account, and channel components SHALL all be
minted locally.

Import SHALL complete strict canonical parsing, bounds, and security preflight
for the entire manifest before Registry mutation. Duplicate entries, canonical
Provider/account collisions, and ambiguous mappings SHALL fail all-or-none;
import SHALL not overwrite, merge, keep both, reuse a source ID, or report
partial success. This guarantee is logical Registry atomicity and SHALL NOT be
read as a physical filesystem atomicity claim. Malformed, over-bound, legacy,
path-bearing, symlink/archive-traversal, duplicate-key,
unknown-security-critical, secret/trust-canary, and noncanonical input SHALL
fail closed before mutation, with key-free public results.

Import SHALL reject every non-terminal Legacy Migration Recovery before
changing Registry revision, without reading or recovering the Secret Store.
The Store port SHALL require its production implementation to journal durable
commit, verify exact readback, and complete the prior-byte rollback evidence
protocol needed across failure and restart. These are logical Registry and restart/readback guarantees and SHALL
NOT claim physical filesystem movement or deletion atomicity.

#### Scenario: Provider configuration is exported
- **WHEN** a user exports settings or Provider configuration
- **THEN** the artifact uses the strict `analytix.provider-portable-manifest/v1` schema
- **AND** it contains only bounded Provider/account descriptors, normalized key-free endpoint/proxy and model/media metadata, manifest-local route correlation, and an explicit per-entry `reentry_required` or `protected_recovery_available` intent
- **AND** it contains no credential bytes, recovery codes, private keys, opaque references, Registry state, source authority IDs, device identity, trust/permission state, storage payload/path, log, or user data

#### Scenario: Provider configuration is imported on another device
- **WHEN** a destination imports the key-free artifact
- **THEN** strict canonical validation completes before any Registry mutation and the destination mints every Provider/account identity and fence/reference
- **AND** it creates uncredentialed, unselected, disconnected, non-executable entries marked `reentry_required`, with routes retained only as local correlation
- **AND** normal protected onboarding or an explicit protected credential-recovery flow must commit and authorize a credential before selection, route admission, or execution

#### Scenario: Portable import collides with destination state
- **WHEN** a manifest contains duplicate entries, a canonical Provider/account collision, or an ambiguous mapping
- **THEN** the destination rejects the complete import before Registry mutation
- **AND** it neither overwrites, merges, keeps both, reuses source identity, nor reports partial success

#### Scenario: Portable import is malformed or security-sensitive
- **WHEN** a manifest is malformed, over-bound, legacy, path-bearing, symlink/archive-traversal, duplicate-key, noncanonical, contains an unknown security-critical field, or carries a secret/trust canary
- **THEN** the destination fails closed before Registry mutation and returns only a key-free error/result

### Requirement: Protected cross-device recovery is explicit, destination-bound, and source-preserving
Closure B SHALL use a separate protected offline protocol only after ordinary
portable import has completed. Ordinary import SHALL produce and Main SHALL
cache a key-free `PortableImportReceipt` containing the exact canonical
manifest bytes plus the bounded correlation-to-destination-minted entry
result and Registry fences. The destination Manager SHALL strict-parse and
re-hash those bytes and cross-check every manifest descriptor, correlation,
destination entry, and fence against the current imported uncredentialed
Provider/account metadata before creating any protected pending state. A
caller-supplied digest paired with an arbitrary entry list SHALL be rejected.
The local Main-owned pending/session record MAY retain the current
BrowserWindow, mainFrame, profile, data-directory, owner bindings, and native
confirmation state, but none of those values SHALL be serialized into an
external request, bundle, or receipt.

The canonical external records SHALL use exact schema/version pairs:
`analytix.provider-protected-recovery-request/v1`,
`analytix.provider-protected-recovery-bundle/v1`, and
`analytix.provider-protected-recovery-receipt/v1`. Every record SHALL be
bounded canonical UTF-8 JSON with exact fields, no duplicate or unknown keys,
no trailing data, no legacy/noncanonical encoding, and no malformed,
path-bearing, symlink/archive-traversal, or security-sensitive material.
Record and named-subrecord digests SHALL be SHA-256 over their exact canonical
UTF-8 bytes.

The destination request SHALL contain only the canonical manifest digest, a
random operation/session identifier and nonce, expiry, the destination
ephemeral X25519 public key, a short verification fingerprint derived only
from the canonical request digest, and the complete ordered
manifest-correlation to destination-minted entry-ID and per-entry Registry
fence map. It SHALL contain no raw profile/data-directory/window/frame value,
device identity, source ID, source fence snapshot, credential, or private key.
The request digest SHALL be computed by the parser as SHA-256 over the
canonical request payload with the derived fingerprint field omitted;
`requestDigest` SHALL NOT be serialized in the request itself. The parser
SHALL verify the fingerprint, and the computed request digest SHALL be carried
by the bundle and receipt. The fingerprint SHALL be the lowercase hex encoding
of the first eight bytes of SHA-256 over that request digest.

The source bundle SHALL contain only the request and manifest digests,
operation/session transcript fields, the ordered source-correlation to
destination-entry map, allowlisted purposes, a source ephemeral X25519 public
key, and one bounded AES-256-GCM envelope over the canonical ordered
credential payload. It SHALL contain no source Provider/account IDs,
credential references, source Registry/K1 snapshot, profile/data-directory or
device binding, selection, trust, permissions, or plaintext. The receipt SHALL
contain only request/manifest/bundle transcript digests, operation/session
fields, expiry, bounded commit/readback status, and the exact correlation to
destination-minted ID/status map plus an exact 32-byte transcript-authentication
tag. The tag SHALL be derived from the transfer key with a domain-separated
receipt-authentication subkey and SHALL bind the canonical receipt transcript
and complete recovery AAD. It SHALL be key-free and contain no path,
profile/window binding, bundle ciphertext, credential, private key, source
authority, or source Registry snapshot.

The source SHALL derive the envelope key from target-bound X25519 using the
destination request public key and a source ephemeral private key. HKDF-SHA-256
SHALL use the request digest as salt and the exact info string
`analytix.provider-protected-recovery/hkdf/v1`; AES-256-GCM SHALL use a
12-byte nonce. AAD SHALL be the exact four-byte-big-endian-length-prefixed
UTF-8 concatenation, in this order: protocol/schema version, request digest,
manifest digest, destination ephemeral public key, source ephemeral public
key, operation/session identifier, operation/session nonce, ordered
source-correlation-to-destination-entry mapping, ordered credential purposes,
and expiry. Mapping and purposes SHALL use their canonical order and byte
encoding. Destination fences are transitively bound by the canonical request
digest and SHALL not be substituted by profile/data-directory or source
Registry snapshots.

The only eligible purposes SHALL be `provider-api-key`,
`provider-oauth-token-bundle` (with subscription data only inside that bundle),
`mcp-oauth-access-token`, and `extension-provider-account-token`. Hub,
authorization-state, transport, and all other credential classes, as well as
trust/permission state, device/install identity, source keys, source IDs, and
opaque references SHALL be rejected. Destination K1 re-encryption is the only
credential commit; selection and route authority SHALL not be transferred.

The Manager SHALL own protected-session authority. The destination Manager's
prepare operation SHALL accept only the exact canonical
`PortableImportReceipt`, an authenticated Main local binding, and the complete
current destination MCP/extension owner-binding inventory. It SHALL validate
the receipt against current Registry state, then generate the
operation/session identifier and nonce, bounded expiry, and ephemeral X25519
key pair, protect the private key under destination K1, and durably store an
unconfirmed pending session. It SHALL return request bytes and only the
computed request/manifest/item-set digests, short fingerprint, and expiry
needed for native confirmation. A separate private destination-confirm
operation SHALL record the exact fresh confirmation and current destination
inventory in the Manager-owned pending session; only a confirmed session may
write the request file. Cancel or failed confirmation SHALL roll back the
unconfirmed pending state. Apply SHALL load the exact confirmed pending
session by request/operation digest from the Manager's Registry/K1 records,
re-resolve the current destination owner inventory, and SHALL reject
caller-supplied session authority, generated key/session values, fences, or
confirmation-consumed state.
The source Manager SHALL derive current source owner, purpose, and fence from
its own canonical manifest and Registry. Its only external authority inputs
SHALL be request bytes, an exact current Main local-action confirmation, and
the complete Main-owned source MCP/extension binding inventory; caller-selected
purposes, source IDs, credential references, and source sessions SHALL not be
authoritative. Finalize SHALL load its own durable source transfer session by
the receipt transcript.

The ordinary Main ceremony SHALL use exactly five operation-specific private
calls—prepare request, confirm destination, create source bundle, apply
destination bundle, and finalize source receipt—with exactly two fresh native
confirmations, three native opens, three native saves, and three writes. Main
SHALL pass only Main-resolved full owner descriptors to prepare, confirmation,
and apply; renderer/public inputs and results SHALL contain no path, file
handle, binding fingerprint, fence, or secret bytes.

#### Scenario: A destination creates and confirms a protected recovery request
- **WHEN** the destination has completed ordinary import and starts the protected offline flow
- **THEN** Main calls its Main-owned `getPortableImportReceipt()` dependency, binds the local protected session to the actual invoke sender, current BrowserWindow/mainFrame/profile/data-directory, imported manifest bytes, destination entry IDs/fences, and current destination MCP/extension owner inventory, and the Manager generates the operation/session nonce, bounded expiry, and destination ephemeral key while storing an unconfirmed pending session
- **AND** Main shows one fresh native destination confirmation over the generated request fingerprint, exact item set, digest, and expiry, records it through a separate private confirmation operation, and only then saves the canonical request through a native dialog to a regular non-symlink path
- **AND** a cancel or failed confirmation rolls back the unconfirmed pending state; no caller-supplied path, fence, digest, binding authority, secret/bundle bytes, or file handle is accepted from renderer/preload

#### Scenario: A source creates a protected recovery bundle
- **WHEN** source Main opens one matching request and resolves current MCP/extension owner bindings from its own inventory
- **THEN** source Main requires the one fresh source native confirmation over the same short request fingerprint, request/manifest digest, exact local operation/profile/window/frame, item set, and expiry
- **AND** the source emits only the allowlisted encrypted bundle, uses one operation-specific private K1 consumer, and does not mutate, disconnect, or delete the source Provider, account, or credential

#### Scenario: A destination applies a protected recovery bundle
- **WHEN** destination Main opens a matching bundle while its confirmed, unconsumed local session remains current
- **THEN** destination re-resolves the exact current destination MCP/extension owner inventory, consumes that confirmation, and rejects replay, expiry, tampering, wrong key, owner/fence drift, missing or incompatible source/destination MCP/extension binding, or partial/ambiguous mapping before mutation
- **AND** it re-encrypts all eligible credentials under destination K1 in one Registry-lock batch, performs CAS and exact readback, and saves only a bounded key-free receipt

#### Scenario: Protected recovery commit fails or restarts
- **WHEN** interruption, restart, commit/CAS/readback failure, currentness drift, or cleanup failure occurs at any phase
- **THEN** the prior committed destination Provider/account/credential state and source Provider/account/credential state remain unchanged until the complete batch is verified, with no partial winner
- **AND** bundle replay is rejected after the pending session is consumed while the verified receipt remains transcript-bound for source finalization and audit
- **AND** a pre-terminal crash/restart preserves the prior committed destination Provider/account/credential authority while retaining recoverable encrypted pending/session/candidate records; the protected Registry/K1 journal state may advance, but pending candidates are not visible, selectable, or executable
- **AND** resume requires fresh destination currentness plus explicit confirmation, otherwise rollback removes only exact candidate/private-key artifacts and retains imported uncredentialed entries and fences

#### Scenario: A source finalizes a verified receipt
- **WHEN** source Main opens a receipt that exactly matches its protected transfer session
- **THEN** it finalizes only that protected session and exact app-owned temporary artifacts, with no additional confirmation prompt and no mutation of the source Provider, account, credential, selection, or authority
- **AND** public, IPC, renderer, preload, logs, and diagnostics contain only bounded status/count/boolean values, never credential, bundle, key, path, or private file data; external or user-copied files are never claimed erased

The production Store SHALL provide the journal/readback/rollback durability
contract required for these logical guarantees; this SHALL NOT be interpreted
as physical filesystem movement or deletion atomicity. Kun's
inspect/preflight-to-staging-to-journal-to-verify/rollback shape is reference
only; its installation, passphrase, signature, trust, or private-store
semantics SHALL not be reused.

The protected journal SHALL validate explicit `pending_key_candidate` (the
pre-K1 journal), `pending`/unconfirmed, `confirmed`, `candidates_durable`, `verification_pending`,
`applied`/receipt-ready, `source_bundle`, `cleanup_pending`,
`rollback_pending`, `finalized`, and `rolled_back` phases. The destination-key
reference and prior key-free fences SHALL be journaled in
`pending_key_candidate` before K1 commit; a failed K1 commit/readback or
pending-publication transition SHALL leave only an exact retryable cleanup
record and SHALL never publish a request. Before the single Provider batch,
all K1 candidates SHALL be committed and exact-byte read back
through the operation-specific protected-recovery consumer. The batch SHALL
record `verification_pending` plus exact candidate references/purposes,
transcript-bound digests, and prior destination metadata/fences sufficient for
deterministic retry or rollback. Before ordinary Snapshot, selection, account
resolution, or execution returns, recovery SHALL reload and verify the exact
successor, fences, owner mapping, receipt transcript, and K1 bytes; a mismatch
SHALL restore the prior logical Registry state. Unverified rollback or cleanup
SHALL retain exact retry metadata and fail closed, and successful terminal
readback SHALL precede cleanup of the ephemeral destination key. A
pre-terminal restart MAY retain encrypted pending/session/candidate records,
but SHALL preserve prior committed Provider/account/credential authority and
shall not expose a pending candidate to selection or execution. Source K1 reads
and source-session publication SHALL use one locked currentness view or an
exact second fence/owner check before returning the bundle.

### Requirement: OAuth authorization uses exact owner bindings and Main-only native callbacks
Provider OAuth SHALL use versioned key-free Provider Registry binding metadata under the current Provider fence. MCP OAuth SHALL use the exact Main-normalized MCP server/account configuration and current config fingerprint, and extension OAuth SHALL use the verified installed plugin/server/account manifest and source fingerprint. Binding or owner-generation changes SHALL invalidate stale authorization, refresh, revocation, and callback work. Bindings SHALL contain only the issuer, normalized authorization/token/optional revocation endpoints, public client identifier, ordered scopes, and code-owned redirect version; confidential clients are unsupported.

#### Scenario: Main begins native authorization
- **WHEN** an operation-specific Provider, MCP, or extension account action begins OAuth authorization
- **THEN** Main commits a protected pending record before opening the external user agent
- **AND** Main derives `com.analytix.desktop:/oauth/callback/<binding-key>` from the exact owner/provider/account/channel/issuer binding
- **AND** renderer and preload receive only a bounded authorization identifier, expiry, and redacted phase without URL, state, nonce, PKCE verifier, callback code, token, or credential reference

#### Scenario: A native callback is routed
- **WHEN** macOS `open-url` or Windows/Linux second-instance argv supplies an OAuth callback
- **THEN** Main accepts only the exact scheme and binding-key path, one unambiguous code/state or bounded error tuple, and the initiating profile/window authority
- **AND** it locates exactly one protected pending record by state plus binding-key and uses only that record's current binding and endpoints

#### Scenario: Callback binding or issuer is mixed up
- **WHEN** a callback is replayed, late, ambiguous, cross-profile, cross-window, cross-binding, has a mismatched issuer, or mixes success and error fields
- **THEN** Main fails closed without a token exchange or credential commit and without selecting any callback- or response-supplied endpoint authority

### Requirement: Ordinary startup is completely Hub-cold
Ordinary Analytix startup SHALL have zero Hub static import, lazy import, service instance, refresh timer, token read, network request, account-readiness gate, model discovery, or gateway fallback. Hub source and compatible legacy data MAY remain only behind an explicit deprecated compatibility or migration entry point.

#### Scenario: Ordinary development or packaged startup runs
- **WHEN** the user does not open a deprecated Hub compatibility or migration surface
- **THEN** no Hub module or service is loaded, no Hub credential is read, no Hub timer or request occurs, and no Hub Provider or fallback is selected

#### Scenario: Deprecated Hub compatibility is opened
- **WHEN** the user explicitly enters the deprecated Hub surface
- **THEN** Analytix may lazy-load only the compatibility adapter needed for that visible action
- **AND** closing or never entering the surface leaves ordinary Provider authority unchanged

### Requirement: Acceptance observes real isolated product seams
Acceptance SHALL exercise real Electron development with an isolated profile and a fresh local-nonpublishable package with a clean profile. It SHALL observe protected onboarding, Registry durability and recovery, key-free surfaces, unified consumer resolution, import/export exclusions, and complete ordinary Hub inactivity without recording real credential values or raw Provider bodies.

#### Scenario: Development acceptance runs
- **WHEN** the development app is launched against an isolated data directory and configured through the product UI
- **THEN** observed settings, renderer/IPC snapshots, Registry metadata, recovery behavior, consumer execution, and Hub inactivity satisfy this specification

#### Scenario: Fresh package acceptance runs
- **WHEN** a fresh local-nonpublishable package is launched with bootstrap disabled and a clean data directory
- **THEN** the package user enters a credential through normal protected UI and the same authority and Hub-cold observations succeed

#### Scenario: Acceptance evidence is retained
- **WHEN** automated or operator evidence is produced
- **THEN** it uses synthetic credential markers or redacted assertions and contains no real keys, tokens, user data, raw Provider bodies, or copied credential storage
