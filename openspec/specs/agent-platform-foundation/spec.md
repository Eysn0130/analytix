# agent-platform-foundation Specification

## Purpose
Define the canonical Analytix Agent Platform brand, the one production Go Agent Harness, the built-in Privacy Layer, least-privilege plugin boundaries, and the role of Funds and later professional plugins.

## Requirements

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

### Requirement: Plugins use typed least-privilege capabilities
Every distributable plugin package SHALL declare an identity, version, lifecycle, contributions, and requested capabilities, and package installation SHALL NOT itself grant runtime authority. The Host SHALL separately grant only typed, scoped, revocable runtime capabilities. A plugin SHALL NOT receive ambient access to DuckDB paths or handles, arbitrary SQL, provider credentials, unrestricted filesystem/shell/network, raw history, authority implementations, or direct output channels.

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
