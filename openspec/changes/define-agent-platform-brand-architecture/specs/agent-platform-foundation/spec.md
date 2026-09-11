## ADDED Requirements

### Requirement: Canonical Agent Platform brand
Analytix SHALL use **Analytix** as the brand name, **Agent Platform** as the English product category, and the approved brand messages without introducing Funds or another domain into the primary product definition.

#### Scenario: English public entry point
- **WHEN** a public English product entry point presents the product category and primary brand message
- **THEN** it identifies Analytix as an `Agent Platform` and uses `Analytix — Agents for sensitive work.`
- **AND** it MAY use `Built for any work. Ready for sensitive work.`, `From everyday workflows to sensitive data.`, and `Any capability. Any model. Private by design.` in their approved roles

#### Scenario: Chinese public entry point
- **WHEN** a public Chinese product entry point presents the primary brand message
- **THEN** it uses `Analytix —— 面向敏感业务而生的通用 Agent 平台。`
- **AND** it MAY use `胜任日常，更胜任敏感业务。` as the subtitle

#### Scenario: User-visible product name
- **WHEN** the desktop, installer, package UI, or another user-visible product surface displays the product name
- **THEN** it uses `Analytix` capitalization
- **AND** canonical package ids, executable names, CLI commands, protocol ids, app ids, and environment variables remain unchanged unless a separate compatibility change is accepted

#### Scenario: Domain capability is described
- **WHEN** Funds or another professional domain is described in product documentation
- **THEN** it is identified as a plugin or capability of Analytix rather than Analytix Core, the primary brand, or the product category

### Requirement: One production Go Agent Harness
Analytix SHALL have exactly one production Agent Harness, implemented by `packages/runtime-go`, and SHALL NOT introduce a second TypeScript Agent runtime, sensitive-work runtime, session family, tool registry, permission system, or authority family.

#### Scenario: Ordinary and sensitive work execute
- **WHEN** an ordinary task and a sensitive professional task are executed
- **THEN** both use the same Go-owned Agent loop, session lifecycle, tool lifecycle, provider gateway, permission model, and public event protocol
- **AND** the professional task receives only additive scoped capabilities

#### Scenario: TypeScript launcher starts runtime
- **WHEN** `analytix serve` starts the production runtime
- **THEN** the TypeScript layer launches the Go `runtime-server` and does not select or implement a second production Agent loop

### Requirement: Canonical architecture expression
Analytix SHALL describe its external architecture as **Go Agent Harness + Plugins + Privacy Layer**, where Privacy Layer and the plugin host are responsibilities inside the one Go Agent Harness rather than independent runtimes.

#### Scenario: Architecture is documented
- **WHEN** a public or normative architecture document summarizes the platform
- **THEN** it uses the canonical three-part expression
- **AND** it explains that the Privacy Layer is built in and cannot be disabled or bypassed by a plugin

### Requirement: Core owns unbypassable platform boundaries
The Go Harness Core SHALL own the Agent loop, sessions, tool and job lifecycle, subagent delegation, provider/model egress, permissions and approvals, sandbox policy, durable/public projection, plugin capability grants, and protected local presentation broker.

#### Scenario: Plugin invokes a model or public channel
- **WHEN** a plugin requests a provider call, durable append, log, telemetry event, export, SSE/WS publication, or protected local display
- **THEN** the request passes through the Core-owned policy and projection boundary
- **AND** a plugin assertion that content is safe does not bypass Core validation

#### Scenario: Plugin is unavailable
- **WHEN** an optional professional plugin is missing, disabled, incompatible, unauthorized, or has a domain-semantic fault
- **THEN** its capabilities are unavailable or degraded
- **AND** ordinary Agent work remains startable and usable unless the shared Core itself has a safety-critical physical fault

### Requirement: Privacy Layer applies complete data-flow projection
The built-in Privacy Layer SHALL classify data and SHALL produce separate private-source, model-safe, public-durable, and protected-local projections rather than sharing one raw message or event representation across consumers.

#### Scenario: Sensitive source reaches a model lane
- **WHEN** DuckDB data, a file, a tool result, an attachment, user input, or a Host/app-server/plugin-injected item contains an account name, identity number, account/card number, address, telephone, private path, raw row, or equivalent sensitive value
- **THEN** every primary-model, retry, subagent, title, compaction, memory, embedding, OCR/vision, guard, evaluation, and background-model lane receives only the admitted model-safe projection
- **AND** no reverse map or raw source handle is included

#### Scenario: Content is persisted or published
- **WHEN** content is written to ordinary history, replay state, SSE/WS, logs, telemetry, exports, CLI ready/startup metadata, diagnostics, crash reports, or support bundles
- **THEN** a positive public/durable projection admits only the allowed fields
- **AND** raw provider bodies, raw tool bodies, PII, private paths, SQL, reverse maps, and unrestricted metadata are omitted

#### Scenario: Sensitive user input must remain locally visible
- **WHEN** a user enters a sensitive source value that must remain available for authorized local display after resume
- **THEN** the Host MAY retain the original segment only as an immutable, scoped private-source artifact with explicit retention
- **AND** ordinary history stores the model-safe projection plus a value-free display binding rather than the raw segment or a reverse map

#### Scenario: Classification is unknown
- **WHEN** a sensitive-path value has no accepted classification or has conflicting classifications
- **THEN** the affected model/public/plugin effect fails closed without disabling unrelated ordinary work

### Requirement: Protected local presentation does not re-enter model or public history
Analytix SHALL display authorized unmasked values only through a Core-owned typed local presentation contract bound to the current principal and applicable authority, and SHALL NOT recover those values from model aliases or ordinary history.

#### Scenario: Desktop UI displays a source-exact value
- **WHEN** an authorized desktop component requests an unmasked value
- **THEN** the Host revalidates the current principal, case or work scope, source snapshot, context epoch, grant, and retained value-free binding as applicable
- **AND** it reads the retained source through a no-store typed local response
- **AND** the value is not emitted through Provider, MCP, SSE/WS, generic renderer state, logs, telemetry, compaction, or ordinary transcript

#### Scenario: CLI displays a source-exact value
- **WHEN** an authorized user explicitly requests unmasked local CLI display
- **THEN** the CLI uses the same protected local presentation contract with masked-by-default output and an interactive local authorization boundary
- **AND** non-interactive, redirected, piped, logged, or unbound requests fail closed

### Requirement: Model history and recovery remain privacy-safe
Analytix SHALL preserve multi-turn semantic continuity using model-safe history and Host-private value-free authority, and SHALL NOT treat model-generated summaries, aliases, or replay text as authority for source-exact values or professional facts.

#### Scenario: Conversation is compacted and resumed
- **WHEN** a session containing sensitive work is compacted, persisted, resumed, forked, or replayed
- **THEN** model-visible context is reconstructed only from admitted safe projections
- **AND** any private authority is revalidated independently at use time

#### Scenario: Work is delegated to a subagent
- **WHEN** a parent Agent delegates sensitive work
- **THEN** the child receives only the minimal model-safe context and capabilities bound to that delegation
- **AND** it does not inherit a raw source handle, reverse map, private carrier, or broader authority by default

### Requirement: Canonical plugin package declaration and exact projections
Except for the bounded current Funds `legacy-v0` migration scenario below, every
Analytix distributable plugin package SHALL use
`.analytix-plugin/package.json` as its only canonical package-plane declaration.
The schema v1 declaration SHALL be a closed JSON object containing exactly
`schemaVersion`, `packageId`, `packageVersion`, `contributions`,
`requestedCapabilities`, and `lifecycle`. Contributions, entrypoints, and
requested capability identifiers SHALL be typed and unique in their applicable
scope; package paths SHALL be normalized package-relative paths that cannot
escape the admitted artifact. The lifecycle declaration SHALL identify only the
required protocol version and entry policy, not current Host state or authority.

`.codex-plugin/plugin.json`, marketplace/UI metadata, MCP declarations, and MCP
server identity/version metadata SHALL be deterministic projections of the
canonical declaration or SHALL pass an executable exact-parity gate against it.
Go-signed receipt/index/ready bindings SHALL carry the admitted declaration's
exact package identity and version. The Host-owned static admission policy SHALL
remain separate and SHALL define supported schema and lifecycle protocols,
first-party identity, entry policy, capability ceiling, artifact integrity,
provenance, signing, and admission constraints without copying the publisher's
exact package version unless an independently justified compatibility rule
requires it.

#### Scenario: Single canonical version edit updates every projection
- **WHEN** a publisher changes `packageVersion` only in the canonical declaration and runs the supported projection and admission workflow
- **THEN** every generated platform, marketplace/UI, MCP, receipt, index, and ready version projection carries that exact version
- **AND** any non-generated consumer literal or stale projection fails an executable parity gate before package startup
- **AND** the Host policy does not become a second owner of the publisher's exact version

#### Scenario: Unknown duplicate or invalid declaration fails closed
- **WHEN** the declaration contains an extra or unknown field, a duplicate contribution, entrypoint, or capability id, an invalid package id or semantic version, an unsupported schema or lifecycle protocol, an invalid entry policy, or an unsafe package path
- **THEN** admission fails before a projection, install state, receipt, MCP startup, or capability effect is accepted
- **AND** the affected Funds capability is disabled while ordinary Agent work remains available when the shared Core is healthy

#### Scenario: Derived projection must be exact
- **WHEN** `.codex-plugin/plugin.json`, marketplace/UI metadata, `.mcp.json`, MCP server metadata, or a typed startup projection disagrees with the admitted canonical identity, version, contribution, or entrypoint
- **THEN** the executable parity or admission gate rejects the package projection
- **AND** no mismatched projection becomes an identity owner or runtime authority

#### Scenario: Declaration requests but does not grant
- **WHEN** an admitted declaration requests one or more Core capabilities
- **THEN** the request is intersected with the independent Host policy and current Host authority before any typed grant can be issued
- **AND** the declaration, platform manifest, install marker, marketplace state, MCP metadata, receipt presence, and package presence cannot mint or widen a grant

#### Scenario: Package lifecycle remains separate from Host lifecycle state
- **WHEN** a package declares a supported lifecycle protocol and entry policy
- **THEN** task 2.1 admission validates those prerequisites without accepting a package-authored `ready`, `healthy`, `granted`, `disabled`, or `revoked` state
- **AND** setup, ready, failed, stopped, grant, health, disable, revoke, and recovery transitions remain Host-owned under task 2.2

#### Scenario: Current Funds package uses bounded legacy migration
- **WHEN** the current first-party Funds package has no canonical companion declaration
- **THEN** an explicit `legacy-v0` adapter may produce an in-memory v1 typed declaration only when the existing Host policy, exact packaged artifact, and Go-signed materialization receipt/index all validate
- **AND** the adapter does not write back the package, trust a user-writable install marker, widen the capability ceiling, or admit a third-party executable plugin

#### Scenario: Legacy or companion admission fails only Funds
- **WHEN** the bounded legacy adapter cannot validate the current Funds artifact or a present companion declaration is missing required data, invalid, unknown, duplicate, or incompatible
- **THEN** Funds remains unavailable without an unsandboxed or manifest-only fallback
- **AND** ordinary Agent startup and work remain available when the shared Core is healthy

### Requirement: Plugins use typed least-privilege capabilities
Every distributable plugin package SHALL declare its identity, version, lifecycle protocol, contributions, and requested capabilities through the canonical package declaration, and package installation SHALL NOT itself grant runtime authority. The Host SHALL separately grant only typed, scoped, revocable runtime capabilities. A plugin SHALL NOT receive ambient access to DuckDB paths or handles, arbitrary SQL, provider credentials, unrestricted filesystem/shell/network, raw history, authority implementations, or direct output channels.

#### Scenario: First-party professional plugin loads
- **WHEN** a first-party professional plugin is registered
- **THEN** its manifest, packaged identity, capability request, version, and health are validated
- **AND** any sensitive native computation is exposed through a Host-owned typed capability rather than an unbounded raw-data API

#### Scenario: Plugin package is installed
- **WHEN** a package contributes a manifest, skill, prompt, MCP declaration, hook, asset, or public-safe UI extension
- **THEN** those contributions become discoverable only after package admission
- **AND** installation or hook execution does not grant sensitive source, raw history, provider, network, persistence, logging, telemetry, or protected-local authority

#### Scenario: Third-party executable plugin loads
- **WHEN** a third-party plugin requires executable code
- **THEN** it runs behind a process isolation boundary with explicit resource and data capabilities
- **AND** it has no sensitive raw-data capability by default

#### Scenario: Required plugin isolation is unavailable
- **WHEN** an executable plugin requires a process or OS isolation boundary that the Host cannot establish
- **THEN** the Host denies or disables that plugin capability instead of running it unsandboxed
- **AND** unrelated ordinary Agent work remains available when the shared Core is healthy

#### Scenario: Plugin attempts direct egress
- **WHEN** a plugin attempts to open an undeclared provider, network, persistence, logging, telemetry, or local-display path
- **THEN** the Host denies the effect and records a value-free decision event

### Requirement: Local non-publishable native execution is separate from release trust
The Go Host SHALL support one strictly bounded source-development native
execution admission by reusing the existing native component receipt, registry,
platform verifier, owner abstractions, and `AuthorityUseLocalBuild`. It SHALL
accept only an exact `development_non_publishable` generation bound to a fresh
isolated profile, synthetic or fixture data, current source closure, toolchain,
target, component hashes, package/runtime identity, and ad-hoc platform policy.
The resulting owner authority SHALL be process-local, non-serializable,
non-exportable, non-promotable, and non-publishable.

This local-build admission SHALL NOT create a second registry, Agent Core,
publication authority, `CargoExecutionReceiptV1`, `PublicationPermitV1`, or
capability-use receipt family. Package presence, a declaration,
`requestedCapabilities`, manifest, marker, environment variable, CLI flag, or
ordinary configuration SHALL NOT mint the native owner or any plugin grant.

#### Scenario: Exact isolated development generation is admitted
- **WHEN** the current local-nonpublishable package, isolated profile, synthetic source, development classification, generation receipt, source/toolchain/target/component hashes, runtime identity, and ad-hoc platform policy all match the independent Host policy
- **THEN** `AuthorityUseLocalBuild` may construct the existing bounded native owner for that process only
- **AND** every plugin operation still requires its separately admitted typed, scoped, revocable Host grant

#### Scenario: Local-build input is absent damaged or stale
- **WHEN** any required classification, profile, source provenance, receipt, hash, component, target, runtime identity, ad-hoc signature, or current Host binding is absent, damaged, mismatched, or stale
- **THEN** local native execution fails closed before the component effect
- **AND** the affected plugin capability becomes unavailable while ordinary Agent work remains usable when shared Core is healthy

#### Scenario: Local-build artifact reaches a release path
- **WHEN** a `development_non_publishable` or `AuthorityUseLocalBuild` artifact is submitted to Developer ID signing, notarization, upload, promotion, publication, release packaging, or formal release acceptance
- **THEN** that path rejects it without converting, reclassifying, or upgrading it to `controlled_release`
- **AND** only the existing `controlled_release_receipt` plus Developer ID or required platform signature and exact packaged identity chain may authorize a formal package

#### Scenario: Local-build evidence is reported
- **WHEN** the isolated development checkpoint passes
- **THEN** the maximum claim remains a focused local-nonpublishable development acceptance
- **AND** Package, Product, Formal RC, notarization, publication, and release remain independently unverified

### Requirement: Funds is the first flagship professional plugin
Funds SHALL be positioned and evolved as Analytix's first flagship professional plugin, proving that the general Agent Platform can enter funds analysis, public-security intelligence analysis, and other sensitive professional work without placing funds-domain semantics in Core.

#### Scenario: Funds capability is presented
- **WHEN** public or technical documentation introduces Funds
- **THEN** it calls Funds the first flagship professional plugin
- **AND** it distinguishes Funds schemas, queries, workflows, skills, UI, and reports from the shared Agent Harness and Privacy Layer

#### Scenario: New professional domain is added
- **WHEN** Knowledge, Legal, Research, Writing, Coding, or another domain capability is added
- **THEN** it uses the same plugin identity, capability, lifecycle, privacy, and failure-isolation contracts
- **AND** its domain semantics are not added to Analytix Core

### Requirement: Architecture claims preserve currentness
Product and architecture documentation SHALL distinguish accepted target, current as-built behavior, implementation gaps, and formal release evidence.

#### Scenario: Private-by-design message is used
- **WHEN** `Any capability. Any model. Private by design.` or an equivalent privacy claim is displayed
- **THEN** it is presented as the technical brand and design principle
- **AND** compliance, certification, release readiness, and complete implementation are not inferred without current artifact-bound evidence
