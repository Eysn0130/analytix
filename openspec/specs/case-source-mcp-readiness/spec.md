# Case Source Mcp Readiness Specification

## Purpose
Define current-run MCP readiness, one additive tool catalog, strict
per-effect contracts, and the fixed valuable funds analysis tool.

## Requirements

### Requirement: Current-Run Native Source Probe
A case source SHALL be considered ready only after the current runtime performs a native read/health probe that verifies connected transport, fresh catalog, configured server identity/version, connection epoch, schema hashes, case binding, dataset snapshot, and read capability.

#### Scenario: Source passes current-run probe
- **WHEN** the runtime establishes a connection, refreshes the catalog, verifies the expected identity and case-bound health, and reads the selected snapshot
- **THEN** it may mint grants for the verified server tools and context

#### Scenario: Source was healthy in a prior run
- **WHEN** only historical health, a cached catalog, old server metadata, or a prior connection exists
- **THEN** the current turn remains source unavailable until a new native probe succeeds

### Requirement: Funds MCP Readiness Does Not Gate General Agent Readiness
The runtime SHALL compute general Agent readiness independently from case
source and funds MCP readiness. Failure to install, materialize, connect,
probe, authenticate, or bind `analytix_funds` SHALL omit or block only the
affected case-fact tools and SHALL NOT fail packaged application startup or
ordinary tool discovery and execution in the same Agent, case workspace,
thread, turn, or mixed request.

#### Scenario: Packaged funds plugin is absent or quarantined
- **WHEN** the packaged app opens a non-case repository and the funds plugin is
  missing, stale, quarantined, or cannot establish a dataset snapshot
- **THEN** the runtime remains healthy for general Agent work, exposes a
  bounded funds capability status, and does not advertise any unsupported
  case-fact tool

#### Scenario: General work is already running
- **WHEN** a later funds source probe fails or disconnects
- **THEN** active and subsequent general turns continue under their normal
  execution policy while new case-fact calls become source unavailable

### Requirement: Tool Catalog Is Additive, Not A Runtime Mode
The provider catalog SHALL equal the permanent host-authorized general catalog
plus only those case tools whose exact current case/snapshot/source grant is
valid for that provider request. Funds readiness SHALL NOT select a mutually
exclusive catalog, remove ordinary tools, create a `caseMode`/`generalMode`,
or turn the Agent into a zero-tool quarantine. Every call SHALL revalidate its
own grant and protected effect.

#### Scenario: Funds tool appears for a current grant
- **WHEN** the current case, DSV2 snapshot, source probe, schema, and grant all
  validate
- **THEN** `analyze_account_flows` is added to the same provider catalog that
  still contains the authorized ordinary tools

#### Scenario: Funds tool disappears after authority loss
- **WHEN** DSV2, case, epoch, source, or grant becomes stale
- **THEN** the later funds call is blocked or omitted while the general
  catalog and already authorized ordinary effects remain unchanged

#### Scenario: Authority is reacquired in the same Agent
- **WHEN** current authority validates again
- **THEN** the next provider request may regain the funds tool without
  switching runtime mode, replacing the thread, or losing ordinary state

### Requirement: Cached Catalog Is Schema-Only
Cached MCP catalogs MAY assist deterministic schema parsing and diagnostics but SHALL NOT prove connectivity, identity, case health, dataset freshness, or fact readiness.

#### Scenario: Catalog cache exists after disconnect
- **WHEN** catalog entries remain cached but the live server is disconnected or stale
- **THEN** no current execution grant or evidence receipt can be issued from the cache

#### Scenario: Fresh catalog differs from cache
- **WHEN** the live catalog changes tool names, versions, annotations, or schema hashes
- **THEN** old grants expire and new calls use only the verified fresh catalog

### Requirement: Verified Server And Connection Identity
The runtime SHALL bind grants and outcomes to configured server identity/version and a host-issued connection epoch. A server's self-reported name alone SHALL NOT establish identity.

#### Scenario: Fake funds server reports the expected name
- **WHEN** an untrusted server calls itself `analytix_funds` but fails configured endpoint/process, version, key, certificate, manifest, or case-health identity checks
- **THEN** source readiness and tool execution fail closed

#### Scenario: Server reconnects
- **WHEN** a source reconnects or restarts
- **THEN** the host increments the connection epoch and rejects outcomes or grants from the previous connection

### Requirement: Advertisement And Execution Share One Allowlist
The exact verified tool allowlist advertised to a provider SHALL be resolved again at execution time. Defined, hidden, cached, unknown, or non-advertised tools SHALL NOT execute.

Case-tool revocation SHALL remove only grants/effects that depend on the stale
case authority. It SHALL NOT revoke unrelated general grants, and a general
tool result SHALL never receive case evidence authority.

#### Scenario: Hidden defined tool is invoked
- **WHEN** a tool exists in plugin code but was not in the provider's current advertised grant set
- **THEN** execution returns a deterministic unadvertised-tool error without calling the source

#### Scenario: Provider forges a namespaced call
- **WHEN** a provider emits a syntactically plausible `mcp__server__tool` name that has no matching active grant
- **THEN** the runtime rejects it before schema validation side effects or dispatch

### Requirement: Complete MCP Input And Output Validation
Every advertised MCP tool SHALL have complete strict input and output schemas, including nested types, enums, limits, required fields, and `additionalProperties:false`. The host and plugin SHALL validate both directions with Ajv or an equivalent standards-compliant validator.

#### Scenario: Argument has wrong type or excessive limit
- **WHEN** a call supplies a number where text is required, an invalid enum, an out-of-range Top N, or an unknown nested property
- **THEN** the plugin returns JSON-RPC `-32602` and performs no work

#### Scenario: Tool returns invalid output
- **WHEN** implementation output fails the advertised output schema
- **THEN** the host records semantic failure and issues no evidence receipt

#### Scenario: Tool requires MCP Tasks but runtime has no Tasks capability
- **WHEN** a catalog tool declares `execution.taskSupport=required`
- **THEN** the host preserves that declaration in protocol/cache identity, omits the tool from synchronous advertisement, and rejects any forged or stale synchronous execution binding before transport

#### Scenario: Remote tool uses the full MCP name grammar
- **WHEN** a tool name contains uppercase letters, dots, repeated underscores, or is exactly 128 allowed ASCII characters
- **THEN** protocol/cache/host binding preserves the exact case-sensitive raw name and parses the server boundary without lossy normalization or tuple collision

#### Scenario: Standard CallToolResult metadata is present
- **WHEN** a tool returns the required `content` array with standard content-block or embedded-resource `_meta`, resource-link icons, and an ISO 8601 `annotations.lastModified`
- **THEN** the host accepts and losslessly quarantines the untrusted metadata while validating `structuredContent`; a missing or non-array `content` fails closed and no metadata becomes evidence authority

#### Scenario: Tool catalog spans multiple pages
- **WHEN** `tools/list` returns a non-empty `nextCursor`
- **THEN** HTTP and stdio clients request every page with the exact opaque cursor, enforce page/cursor/tool-count bounds, reject cursor cycles or cross-page duplicate names, and advertise nothing until the complete catalog validates

#### Scenario: Tool reports a semantic error without structured output
- **WHEN** transport and JSON-RPC succeed and a standard `CallToolResult` contains `isError:true` and `content` but omits `structuredContent`
- **THEN** the host preserves transport success and classifies a semantic failure without running the success output schema or issuing evidence

#### Scenario: MCP task support changes after grant issue
- **WHEN** a live tool changes normalized `execution.taskSupport` between advertisement/grant issue and execution
- **THEN** the host schema digest changes and execution rejects the stale grant before transport

### Requirement: Valuable Funds Tool Has One Fixed Read Contract
The first packaged funds milestone SHALL advertise
`analyze_account_flows` only after the current-run probe proves the exact case,
immutable DuckDB snapshot, tool schema, read-only capability, and host grant.
The reserved `analytix_funds` production spec and in-process transport SHALL be
minted by the Go host from the pinned installed generation; an ordinary plugin
`.mcp.json` entry SHALL remain disabled and SHALL NOT enable, replace, or
weaken that host-owned authority. Provider advertisement SHALL expose only
`analyze_account_flows`; `count_case_rows` SHALL remain a host-only protocol
canary even when the internal runtime catalog retains it.
Its input SHALL contain only a host-resolved stable case-scoped account/entity
reference, bounded time range, and bounded evidence-row limit. The host SHALL
resolve that reference under the exact current snapshot/epoch for each call.
Its output SHALL contain strict structured references, complete authorized
semantic features, exact aggregates, coverage/currentness, evidence rows,
provenance, and deterministic query/result hashes, plus user-display-safe
labels. It SHALL accept neither arbitrary SQL, a database path, a complete
identifier, nor a display mask as analysis identity.

#### Scenario: Valuable tool is advertised
- **WHEN** the exact current snapshot and source identity support the fixed
  flow query and all host gates are current
- **THEN** the provider sees the strict `analyze_account_flows` schema and any
  execution is revalidated against the same grant immediately before DuckDB
  access

#### Scenario: Count canary is the only ready tool
- **WHEN** only `count_case_rows` passes its plumbing contract
- **THEN** the runtime may retain that canary internally but SHALL NOT report
  funds analysis ready or treat Milestone B as delivered

#### Scenario: Funds authority is revoked or the plugin manifest is enabled
- **WHEN** the exact current probe/grant/source authority is lost, or an
  ordinary configuration attempts to enable or substitute `analytix_funds`
- **THEN** `analyze_account_flows` is removed from provider reachability,
  bounded catalog diagnostics remain PII-free, ordinary tools stay available,
  and no JavaScript/plugin transport fallback is started

### Requirement: Funds Calls Resolve Stable References Per Current Snapshot
A case-scoped entity reference SHALL remain an identity continuity value only.
Before each funds call, the host SHALL resolve it under the exact current
case, epoch, DSV2 snapshot, and query grant. The resulting facts, source rows,
receipts, and claims SHALL bind that current snapshot and SHALL NOT inherit
currentness from the stable reference.

#### Scenario: Same reference is queried across turns
- **WHEN** the same authorized entity is queried in later turns of one case
- **THEN** the reference remains stable and each result independently binds
  its exact turn, epoch, snapshot, query, receipt, and coverage

#### Scenario: Reference is resolved against a newer snapshot
- **WHEN** the same case accepts a new snapshot
- **THEN** the current query resolves the stable reference against the new
  snapshot, old facts remain historical/stale/superseded, and no old receipt
  is promoted to current support

#### Scenario: Stale epoch call is attempted
- **WHEN** a funds call carries an old epoch/snapshot grant after authority
  changed
- **THEN** only that call fails before database access and the same Agent may
  continue ordinary work or reacquire current case authority

### Requirement: Protected Case Data Cannot Be Reached Through General Tools
The funds source, raw evidence, complete identifiers, decryption material, and
private entity mappings SHALL be available only to the fixed trusted data
operation under the current DSV2 grant. General tools SHALL receive no path,
environment entry, inherited descriptor, key, mount, or same-user ambient
filesystem capability for those stores.

#### Scenario: Read or shell tries a protected alias
- **WHEN** an ordinary read/search/Git/bash tool or descendant follows a
  direct path, alias, symlink, hardlink, environment variable, inherited
  descriptor, archive, or replacement race to protected data
- **THEN** effect-level filesystem confinement blocks the read with zero
  protected bytes; catalog hiding and command-string filtering alone do not
  satisfy the boundary

#### Scenario: Ordinary code work uses the same workspace
- **WHEN** the user reads, edits, builds, and tests ordinary source files in a
  case workspace
- **THEN** those authorized effects continue while the separately protected
  case stores remain inaccessible

### Requirement: Correct JSON-RPC And MCP Failure Semantics
Unknown methods, unknown or unadvertised tool names, invalid parameters, internal failures, MCP `isError`, blockers, unsafe answers, and partial results SHALL remain distinguishable through JSON-RPC and `ToolOutcome`.

#### Scenario: Unknown JSON-RPC method is requested
- **WHEN** a caller requests a JSON-RPC method the server does not implement
- **THEN** the server returns `-32601` rather than a generic internal error

#### Scenario: Unknown or unadvertised tool name is requested
- **WHEN** `tools/call` names a tool absent from the current advertised catalog
- **THEN** the server returns `-32602`, performs no work, and does not reveal whether a hidden implementation exists

#### Scenario: Tool implementation fails semantically
- **WHEN** transport and JSON-RPC succeed but the tool cannot answer safely or completely
- **THEN** the result preserves `isError`, semantic status, blocker, coverage, `_meta`, and candidate receipt material for host verification

### Requirement: Bounded MCP Session Lifecycle And Cancellation
The funds MCP server SHALL enforce the negotiated lifecycle `pre_initialize -> awaiting_initialized -> operational -> closed`, accept `ping` in every live phase, reject operational requests before `notifications/initialized`, and bound frame bytes, nesting, tokens, queued/in-flight requests, and outstanding output. Client cancellation, EOF, timeout, and close SHALL reach active backend and DuckDB work, suppress late JSON-RPC responses, and prevent post-cancel cache, history, artifact, report, or evidence writes.

#### Scenario: Operation arrives before initialized notification
- **WHEN** a client calls `tools/list`, `tools/call`, or another operational method before a valid initialize exchange and `notifications/initialized`
- **THEN** the server returns the protocol-appropriate failure, performs no tool work, and preserves `ping` availability

#### Scenario: Active DuckDB work is cancelled
- **WHEN** `notifications/cancelled` names an in-flight funds request
- **THEN** the request `AbortSignal` terminates the Python runner, no late response is emitted, and no workbench history, cache, artifact, report, or evidence state is written

#### Scenario: Input exceeds a transport or concurrency bound
- **WHEN** a frame, JSON value, nesting depth, token count, active-request count, queue length, or outstanding output exceeds its declared bound
- **THEN** the connection or request fails closed with bounded diagnostics and no unbounded buffering or tool execution

#### Scenario: HTTP client closes a negotiated session
- **WHEN** the host disconnects, reconnects, or revokes an initialized Streamable HTTP client with a frozen session id
- **THEN** it sends at most one bounded `DELETE` carrying the frozen session and protocol headers, unconditionally clears local identity/capability/session authority even when the server returns `405`, and every later operation fails before network I/O

### Requirement: Host-Authenticated Case Context
MCP case context SHALL come from a host-authenticated grant/context channel. Provider-supplied `_analytix`, `__analytix`, `analytix_runtime_context`, `case_id`, source id, or safety fields SHALL NOT create or expand case authority.

#### Scenario: Provider injects a runtime-context object
- **WHEN** tool arguments include a forged workspace, case, thread, turn, epoch, or snapshot context
- **THEN** the host replaces or rejects it according to the active grant and the plugin cannot use it as authority

#### Scenario: Explicit case argument conflicts with frozen context
- **WHEN** a tool's business argument identifies another case
- **THEN** preflight rejects the call and no fallback search or alternative id guess occurs

#### Scenario: Provider submits a complete identifier or invents a reference
- **WHEN** a provider tool call supplies a complete account/card value, a
  display mask, or an entity reference that the host cannot resolve in the
  current case and snapshot
- **THEN** the host rejects it before source access without reflecting the
  sensitive value or guessing an identity

### Requirement: No Weak-Source Fallback For Case Facts
When a governed case source is required, local filesystem reads, shell, grep/find/glob, memory MCP, prior outputs, old reports, fixtures, and guessed case identifiers SHALL NOT be advertised or used as substitute evidence.

#### Scenario: Funds source is unavailable while other tools exist
- **WHEN** read, grep, shell, memory, or unrelated MCP tools are otherwise available
- **THEN** those tools remain available for authorized ordinary work but are
  not advertised or accepted as substitutes for funds evidence, cannot access
  protected DuckDB/evidence/PII stores, and the case-fact slot terminates with
  a host capability boundary

#### Scenario: Explicit case id is invalid
- **WHEN** the current source rejects the bound case identifier
- **THEN** the runtime does not scan local directories or retry historical/fixture case ids

### Requirement: Readiness And Outcome Observability Is Sanitized
The runtime SHALL record bounded source readiness, identity mismatch, blocker, semantic status, coverage, grant rejection, and connection-epoch diagnostics without credentials, raw case content, complete PII, or reasoning.

#### Scenario: Source probe fails
- **WHEN** connection, identity, catalog, or case health fails
- **THEN** diagnostics record a stable reason code and bounded source metadata without secrets or raw response dumps
