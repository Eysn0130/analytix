# Analytix Agent Platform brand and architecture

- Status: Accepted product and architecture target
- Current as of: 2026-08-26
- Applies to: public brand copy, product category, runtime architecture, Privacy Layer, plugin boundaries, and professional capability positioning
- Accepted scoped requirements: [`../../../openspec/specs/agent-platform-foundation/spec.md`](../../../openspec/specs/agent-platform-foundation/spec.md)
- Dated research evidence: [`../upstreams/agent-platform-architecture-recheck-2026-08-25.md`](../upstreams/agent-platform-architecture-recheck-2026-08-25.md)

This spec defines what Analytix is becoming. It does not by itself prove that the current worktree, desktop package or release artifact implements every requirement. Current code and fresh validation remain the source of truth for as-built behavior.

## 1. Canonical product position

| Role | Canonical expression |
| --- | --- |
| Brand | **Analytix** |
| Product category | **Agent Platform** |
| Primary English brand line | **Analytix — Agents for sensitive work.** |
| Primary Chinese brand line | **Analytix —— 面向敏感业务而生的通用 Agent 平台。** |
| English subtitle | **Built for any work. Ready for sensitive work.** |
| Chinese subtitle | **胜任日常，更胜任敏感业务。** |
| Supplemental expression | **From everyday workflows to sensitive data.** |
| Technical brand line | **Any capability. Any model. Private by design.** |
| Architecture expression | **Go Agent Harness + Plugins + Privacy Layer** |

`Agent Platform` is the English product category. Do not replace it with `General-purpose Agent Platform` in public category labels. The Chinese primary line intentionally retains “通用 Agent 平台” as approved copy.

`Analytix` is the user-visible brand lockup. Lowercase `analytix` remains the canonical package, executable, CLI, protocol and environment identity where existing machine contracts require it; the marketing capitalization does not rename those contracts.

`Private by design` describes the product direction and architecture principle. It is not, without current artifact-bound evidence, a claim of compliance certification, perfect anonymity, completed implementation or formal release readiness.

## 2. Product definition

Analytix is an Agent Platform for ordinary and sensitive work. It should provide the general Harness abilities associated with leading coding and work agents—long-running turns, tools, files, shell and controlled execution, planning, todos/goals, skills, MCP, subagents, background jobs, compaction, resume/replay, multi-model providers and desktop/CLI operation—without becoming a coding-only product.

Its distinguishing property is not a separate “trusted mode.” It is that the same general Agent Harness has a built-in Privacy Layer that controls how sensitive source data reaches models, histories, plugins, public channels and local presentation.

## 3. Canonical architecture

```text
Desktop / CLI / API
        |
        v
+-----------------------------------------------------------+
| One Go Agent Harness                                      |
|                                                           |
|  Agent execution core                                     |
|  sessions · tools · jobs · subagents · provider gateway   |
|  permissions · approvals · sandbox · recovery             |
|                                                           |
|  Built-in Privacy Layer                                   |
|  classification · projection · audit · local presentation |
|                                                           |
|  Capability / Plugin Host                                 |
|  Funds · Knowledge · Legal · Research · Writing · Coding  |
+-----------------------------------------------------------+
```

The diagram is one runtime, not three services. `packages/runtime-go` remains the only production Agent core. The TypeScript `analytix serve` layer remains a launcher and public-contract boundary, not a second Agent implementation.

Privacy Layer is part of the Harness safety floor. A plugin cannot unload, replace or bypass it. Plugins extend professional capability; they do not own the final Provider, persistence, permission, log, telemetry, public event or protected local-display boundary.

`Plugin` has two related but non-equivalent planes. A distributable **plugin package** may contain a manifest, skills, prompts, MCP declarations, hooks, assets and public-safe UI contributions. A **runtime capability** is a typed, scoped, revocable grant issued and enforced by the Go Core. Installing or loading a package only makes declared contributions discoverable; it never grants sensitive source access, raw history, direct Provider/network/persistence/logging access or protected local display. Package hooks and MCP servers are untrusted extension inputs, not the final authorization or privacy boundary.

## 4. Core and plugin ownership

| Go Harness Core owns | Plugin owns |
| --- | --- |
| Agent loop, turns, session/history lifecycle, tools, jobs and subagents | Domain tools, workflows, skills/prompts and provider/public-safe UI extensions |
| Provider/model gateway and every model egress projection | Domain schemas, query plans, classifiers and canonicalizers behind typed contracts |
| Permission, approval, sandbox, timeout, cancellation and network/file policy | Integrations requested through declared capabilities |
| Durable/public event schemas, replay, logs, telemetry, export, support-bundle projection and unmasked local renderer | Domain reports, public-safe presentation templates and requests to use the protected display component |
| Plugin identity, admission, capabilities, lifecycle, revocation and isolation | Plugin health and domain-specific degraded-state explanation |
| Current principal/work/case/snapshot/context authority and typed local display | A request to use a scoped Host capability; never the authority implementation itself |

No plugin receives ambient DuckDB paths or handles, arbitrary SQL, provider credentials, unrestricted filesystem/shell/network, raw history, Host-private carriers, reverse maps or direct output channels.

For the first production iteration, first-party professional plugins may be statically registered and fixed in the package. Plugin architecture means one manifest/capability/lifecycle contract; it does not require loading arbitrary dynamic Go code. Third-party executable plugins must be isolated out of process before they can receive high-risk capabilities.

If the required process or OS isolation is unavailable, the affected executable-plugin lane fails closed. Analytix does not silently execute that plugin unsandboxed; unrelated ordinary Agent work remains available when the shared Core is healthy.

## 5. Privacy Layer data flow

Analytix must create separate projections for separate consumers:

| Projection | Purpose | Rule |
| --- | --- | --- |
| Private source | Host-authorized DuckDB/file/source computation; retained sensitive user-input artifact when needed | Raw values stay inside the bounded source authority |
| Model-safe | Main model, retries, subagents, title, compaction, memory, embedding, OCR/vision, guard/eval | Use scoped aliases, exact non-PII facts and bounded safe text; never include raw identity data or reverse maps |
| Public durable | Ordinary history, replay, SSE/WS, logs, telemetry, export, diagnostics and support bundles | Positive schema projection only; omit PII, paths, raw provider/tool bodies, SQL and private authority data |
| Protected local | Authorized Electron UI or local CLI | Revalidate current authority and read retained source through a typed no-store display path |

The UI's ability to display an unmasked account name, identity number, account/card number, address or telephone does not authorize that value in the model prompt or generic UI state. Source-exact display is resolved from the retained source using a host-verifiable value-free binding; it is never reverse-generated from a model alias.

Local CLI unmasked output must reuse the protected display contract, remain masked by default, require explicit interactive authorization, and fail closed for pipe/redirect/non-interactive paths until a separate accepted CLI contract proves those paths safe.

## 6. Multi-turn, context and memory rules

- Ordinary model history stores the admitted semantic projection and value-free display binding, not the raw sensitive source. A sensitive user-input segment that must survive resume is retained only as a scoped private-source artifact and is re-read through protected local presentation.
- Every Provider call reprojects its complete request, including system/user messages, Host/app-server/plugin-injected items, tool schemas, tool arguments/results, attachments and retry context. An item being persisted, injected by the Host, or supplied by a hook is not proof that it is model-safe.
- Title generation, compaction, summaries, memory, retrieval/embedding, subagent context, OCR/vision and evaluation are model lanes and receive the same privacy policy.
- A child Agent receives only the minimal context and capabilities signed for that delegation; parent raw-data handles and reverse maps do not propagate.
- Resume, replay and fork revalidate private authority at use time. Model summaries and aliases never become authority for source-exact display or professional facts.
- Logs and telemetry record value-free decision codes, policy/version, counts and digests, not raw content.

## 7. Funds and future professional plugins

**Funds is Analytix's first flagship professional plugin.** It proves that the general platform can extend from everyday workflows into funds analysis, public-security intelligence analysis and other high-sensitivity professional work.

Funds owns funds-domain schemas, query plans, analysis tools, evidence workflows, skills, provider-safe UI extensions and report templates. It may request the Core-owned protected local component for source-exact display, but arbitrary plugin UI code does not receive the raw value. Funds does not own or define the common Agent loop, Provider gateway, Privacy Layer, permission system, session system or plugin lifecycle.

Knowledge, Legal, Research, Writing, Coding and later professional abilities must use the same plugin identity, capability, privacy, lifecycle and failure-isolation model. No professional domain becomes a mode switch or a second Agent. Missing or damaged professional capability closes only that plugin lane; ordinary Agent work remains available unless the shared Core itself has a safety-critical fault.

## 8. Upstream adoption policy

Analytix selectively absorbs:

- Codex's Harness/Host split, agent loop, sandbox/approval, package-level plugin composition and extension layering;
- Claude Code's built-in tool foundation, permission/sandbox depth and scoped extension model;
- DeepSeek Harness's capability seams, composition, typed append-only events, tool pipeline and resumable subagents;
- OpenCode's explicit session-to-model lowering, lifecycle and permission UX.

Analytix does not adopt a replaceable privacy policy, same-process arbitrary plugins as a sandbox, raw durable history as model context, fail-open sandbox fallback, or plugin-owned telemetry redaction. External code, prompts, skills and assets remain subject to commit pinning, license, provenance and admission review.

## 9. As-built, target and gap

### As-built snapshot rechecked on 2026-08-26

- `packages/runtime-go` is the only production Agent core; Electron/TypeScript uses the Go HTTP/SSE boundary.
- The codebase already contains provider-safe projections, Host-private tool-result handling, positive public event projections, case/evidence authority, and typed local display seams.
- `plugins/analytix-fund-analysis` exists as the Funds plugin source, with a current Go-host-controlled production capability slice.

### Accepted target

- One complete general Go Agent Harness, one Privacy Layer coverage contract, and one plugin identity/capability/lifecycle model.
- Funds and later professional capabilities run through that model without changing ordinary Agent behavior or adding another authority family.

### Current gaps

- Current package-level extension surfaces and privileged runtime capabilities are not yet unified under one general plugin identity/lifecycle plus Core-owned grant contract.
- General `package.json` and Electron builder metadata still use lowercase `productName`; user-visible capitalization must be aligned under packaging validation without renaming lowercase machine identities. Official Standard Windows already retains its accepted localized `Analytix灵鉴` display-name exception.
- Funds-specific tool names, effect classes, output types and local readers still appear directly in shared Go composition; they must move behind registered typed capabilities while preserving historical event and replay compatibility.
- Some optional case/evidence initialization and recovery remain coupled to ordinary runtime startup.
- Privacy coverage still needs one executable matrix across every model, persistence, public, telemetry, export, UI and CLI lane.
- The current Vision bridge can place raw `image.DataBase64` into a Provider message before a trusted local inspector/projector exists; this is an as-built Privacy Layer blocker, not merely missing documentation or an unrun acceptance row. Until fixed, the affected image effect must be treated as unavailable and both primary and bridge model calls must remain zero for unprojected images.
- The current `analytix serve` ready projection includes configured `configPath`, `dataDir` and `durableRoot`; those local-path fields still need an explicit consumer classification and positive projection before a complete public/durable privacy claim.
- Third-party executable plugin isolation and protected CLI source-exact display are targets, not current proved production surfaces.
- Formal source/package/Electron/Provider/recovery/leakage evidence remains required before release claims.

## 10. Non-negotiable acceptance invariants

Future implementation is conformant only when it proves all of the following:

1. There is no second production Agent loop, session store, tool registry, permission system or sensitive authority family.
2. Ordinary Agent tasks pass with Funds absent, disabled, unauthorized and domain-corrupt.
3. Hostile PII/path/raw-body/reverse-map canaries are absent from serialized Provider, history, compaction, subagent, SSE/WS, renderer-generic, logs, telemetry, export and support-bundle outputs.
4. Authorized source-exact UI/CLI display works only through a typed protected local contract and does not alter model/public projections.
5. A plugin cannot widen its own capabilities or bypass final Core authorization/projection.
6. Installing a plugin package does not itself grant sensitive data, raw history, direct egress or protected-local authority.
7. A required plugin isolation boundary that is unavailable closes that plugin lane rather than falling back to unsandboxed execution.
8. Plugin identity/version/revocation/fault recovery is deterministic and current.
9. The same formal artifact proves ordinary workflows and sensitive professional workflows; focused source tests do not substitute for product or release acceptance.

## 11. Supersession and terminology

For public product positioning, Core/Plugin ownership and the role of Privacy Layer, this spec supersedes older descriptions that frame Analytix primarily as a desktop workbench, Agent OS, Funds system or stacked runtime architecture. Those documents remain valid for their historical evidence and narrower implemented contracts.

Use **Agent Platform** for the category, **Go Agent Harness** for the execution core, **Privacy Layer** for the built-in data-protection boundary, and **professional plugin** for domain expansion. Do not use `caseMode`, `generalMode`, `trusted runtime branch`, `Funds core`, or “everything is a plugin” as the Analytix architecture.
