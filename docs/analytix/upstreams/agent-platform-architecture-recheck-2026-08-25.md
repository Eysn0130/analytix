# Agent Platform architecture recheck — 2026-08-25

- Status: Dated upstream research evidence
- Revalidated: 2026-08-26; see Section 8 for the latest-ref delta
- Research question: which current Agent/Harness mechanisms should define the Analytix long-term architecture?
- Product authority: this report supports, but does not replace, the accepted target in
  [`../specs/11-agent-platform-brand-and-architecture.md`](../specs/11-agent-platform-brand-and-architecture.md)
- As-built and release authority: current code, tests, packages, and fresh runtime evidence

## 1. Method and evidence boundary

This review used Exa to inspect **96 results across four search workstreams** and retained official documentation or official repositories for architectural claims. Repository heads were pinned again on 2026-08-25:

| Upstream | Audited ref | Role in this review |
| --- | --- | --- |
| OpenAI Codex | [`34c5303f49d08a5a41294e2531d1e64b40c0302d`](https://github.com/openai/codex/tree/34c5303f49d08a5a41294e2531d1e64b40c0302d) | Agent loop, host/harness split, sandbox, approvals, skills/plugins, subagents |
| Anthropic Claude Code | [`1af51fd77b7e40017c1032249db75fe582d23283`](https://github.com/anthropics/claude-code/tree/1af51fd77b7e40017c1032249db75fe582d23283) | Built-in tool foundation, permission/sandbox, skills/hooks/MCP/agents |
| DeepSeek Harness | [`b150a551b8d465e31e418e1b2eaf5e79bbb7d28e`](https://github.com/deepseek-ai/deepseek-harness/tree/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e) | Capability seams, plugin composition, event log, tool pipeline, subagents |
| OpenCode | [`8615731d46153dd29b89e205fb55b2cc16205cb0`](https://github.com/anomalyco/opencode/tree/8615731d46153dd29b89e205fb55b2cc16205cb0) | Durable sessions, model-message lowering, compaction, plugins and permissions |

The review is architectural and source-based. It did not execute upstream real-model calls and does not certify an upstream product, Analytix implementation, package, or release. Upstream default branches move quickly; implementation work must pin again.

## 2. Cross-upstream result

The common stable pattern is not “put every component behind the same plugin interface.” It is:

```text
stable host/harness-owned execution and policy
                +
explicit extension/capability seams
                +
consumer-specific projection of session and tool data
```

Codex and Claude Code place skills/plugins/MCP/subagents above built-in execution, permission, and sandbox foundations. DeepSeek Harness offers the strongest general-purpose composition model, but deliberately removes a privileged core. OpenCode exposes useful session and plugin mechanisms, while its raw history and in-process plugin surfaces show why permission rules alone are not a privacy layer.

For Analytix, the correct synthesis is therefore:

> **One Go Agent Harness with an unbypassable built-in Privacy Layer and a capability-based Plugin System.**

The public expression remains shorter:

> **Go Agent Harness + Plugins + Privacy Layer**

## 3. Upstream findings

### 3.1 Codex

Official sources:

- [Codex as a platform](https://developers.openai.com/blog/codex-as-a-platform)
- [Unrolling the Codex agent loop](https://openai.com/index/unrolling-the-codex-agent-loop/)
- [Sandboxing](https://developers.openai.com/codex/concepts/sandboxing/)
- [App-server protocol at the pinned commit](https://github.com/openai/codex/blob/34c5303f49d08a5a41294e2531d1e64b40c0302d/codex-rs/app-server/README.md)

Adopt:

- the Harness owns turns, items, tools, conversation state, compaction and resumability;
- the host application owns product context, business rules, interface, tools and operational boundaries;
- OS sandbox and approval policy are distinct controls;
- skills, plugins, MCP and subagents extend a stable execution foundation.

Do not infer:

- a skill, plugin directory or MCP server is a security boundary;
- approval can replace technical sandboxing or field-level privacy projection;
- an unsandboxed user shell is suitable as a model-reachable sensitive capability.

### 3.2 Claude Code

Official sources:

- [How Claude Code works](https://code.claude.com/docs/en/how-claude-code-works)
- [Permissions](https://code.claude.com/docs/en/permissions)
- [Sandboxing](https://code.claude.com/docs/en/sandboxing)
- [Plugins reference](https://code.claude.com/docs/en/plugins-reference)
- [Subagents](https://code.claude.com/docs/en/sub-agents)

Adopt:

- built-in tools as the stable foundation, with skills, hooks, MCP and subagents above it;
- explicit allow/ask/deny rules and sandboxing as defence in depth;
- separate child contexts and explicit tool scopes;
- deterministic lifecycle hooks only as additional checks.

Reject for sensitive authority:

- fail-open sandbox fallback;
- permission bypass as a plugin or subagent default;
- local plaintext session/memory as the place for raw case PII;
- model-evaluated hooks or prompt instructions as a deterministic privacy gate.

### 3.3 DeepSeek Harness

Official pinned sources:

- [README](https://github.com/deepseek-ai/deepseek-harness/blob/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e/README.md)
- [Architecture](https://github.com/deepseek-ai/deepseek-harness/blob/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e/docs/architecture.md)
- [Session telemetry](https://github.com/deepseek-ai/deepseek-harness/blob/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e/docs/subsystems/session-telemetry.md)
- [Persistence](https://github.com/deepseek-ai/deepseek-harness/blob/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e/docs/subsystems/persistence.md)
- [Filesystem policy warning](https://github.com/deepseek-ai/deepseek-harness/blob/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e/packages/fs/README.md)

Adopt:

- Service Definition / Provider / Consumer capability seams;
- boot-time profile/bundle composition;
- an append-only typed event source with explicit model-history derivation;
- pre/execute/post tool pipelines, resumable child sessions, checkpoints and recovery;
- capability failure isolation.

Reject or constrain:

- literal “no privileged core” for privacy, authority, persistence and egress policy;
- same-process dynamic plugins or external MCP executables as a strong sandbox;
- a policy plugin whose absence can leave an unconstrained provider;
- telemetry that forwards complete event payloads unless deployment adds redaction.

The repository labels itself a developer preview with compatibility-breaking changes. That is acceptable for mechanism research, not for delegating the Analytix product contract.

### 3.4 OpenCode

Official sources:

- [Plugins](https://opencode.ai/docs/plugins/)
- [Permissions](https://opencode.ai/docs/permissions/)
- [Custom tools](https://opencode.ai/docs/custom-tools/)
- [Plugin API at the pinned commit](https://github.com/anomalyco/opencode/blob/8615731d46153dd29b89e205fb55b2cc16205cb0/packages/plugin/src/index.ts)
- [Model-message lowering at the pinned commit](https://github.com/anomalyco/opencode/blob/8615731d46153dd29b89e205fb55b2cc16205cb0/packages/core/src/session/runner/to-llm-message.ts)
- [Compaction at the pinned commit](https://github.com/anomalyco/opencode/blob/8615731d46153dd29b89e205fb55b2cc16205cb0/packages/core/src/session/compaction.ts)

Adopt:

- explicit lowering from durable session objects to model messages;
- durable session/message/part structures and compaction events;
- tool/path/command permission UX;
- plugin lifecycle, scope and disposal concepts.

Reject for Analytix privacy:

- raw user text, files, shell output, tool input/result and reasoning flowing from the same history into a model;
- compaction serializing raw conversation/tool content into another model request;
- in-process plugins receiving raw messages, tool arguments, client SDK, worktree and shell access;
- treating allow/ask/deny as data classification, de-identification or output projection;
- session sharing or export without a separate explicit privacy gate.

## 4. Final architecture ruling

### 4.1 What stays in the privileged Go Harness Core

- Agent loop, sessions, tool execution, jobs, subagents, recovery and compaction lifecycle;
- provider/model gateway and the final projection for every model lane;
- permissions, approvals, sandbox policy, timeouts, cancellation and egress;
- public/durable event schemas, history projection, logs, telemetry, export and support bundles;
- plugin identity, admission, capability grants, isolation, revocation and failure containment;
- protected local display broker and current principal/work/case/snapshot/context authority;
- generic evidence/currentness/publication capabilities that contain no Funds-domain semantics.

### 4.2 What belongs in plugins

- domain tools, schemas, workflows, skills/prompts, renderers and report templates;
- domain-specific query plans, classifiers and canonicalizers behind typed contracts;
- integrations that use declared, bounded capabilities;
- Funds, Knowledge, Legal, Research, Writing and Coding product surfaces.

Plugins cannot own or bypass the final Provider, persistence, log, telemetry, public event or unmasked local-display boundary. A same-process plugin is trusted host code, not a sandbox. Third-party executable plugins require process isolation before receiving high-risk capabilities.

### 4.3 Why Privacy Layer is not another branch

Privacy Layer is a cross-cutting policy inside the one Harness. It produces a model-safe view, a public/durable view and an authorized local-display view from the same host-owned source authority. It does not add a second Agent, database, session log, token vault, permission registry or receipt family.

This preserves ordinary Agent quality: when Funds or another domain plugin is absent, corrupt or unauthorized, only that capability closes. The common Harness remains available for ordinary work.

## 5. Current Analytix reality and gaps

Observed at the audited worktree:

- `packages/runtime-go` is already the one production Agent core and the TypeScript layer is a launcher/contracts boundary;
- current source includes provider-safe projection, host-private tool results, positive public event projection and typed local source-exact display seams;
- Funds already has a plugin source tree and a Go-host-controlled production slice.

The target is not yet a completion claim. Important remaining gaps include:

- no single general plugin identity/capability/lifecycle contract yet governs all first-party professional abilities;
- general `package.json` and Electron builder metadata still use lowercase `productName`; visible capitalization needs packaging validation while lowercase machine identities remain stable, and Official Standard Windows retains its accepted `Analytix灵鉴` exception;
- Funds-specific tool names, logical effects, output types and local readers still appear directly in shared Go composition and must migrate behind registered typed capabilities without breaking historical replay;
- parts of case/evidence startup and recovery remain coupled to ordinary runtime startup;
- all model lanes, compaction, memory, embeddings, telemetry, exports and local CLI display still need one audited Privacy Layer coverage matrix;
- the current Vision bridge still places raw image base64 into a Provider message without the accepted trusted projector, so image privacy is an implementation blocker and not a completed lane;
- the current `analytix serve` ready projection exposes configured local paths and still needs explicit consumer classification and positive projection;
- third-party executable plugin isolation and revocation are not a proved production boundary;
- current source, package, Electron, Provider and formal sensitive-data evidence remain separate acceptance tracks.

## 6. Rejected routes

1. **Go Runtime + a separate trusted Agent branch** — duplicates core state and creates ordinary/sensitive drift.
2. **TypeScript general Harness + Go sensitive sidecar** — restores two runtime authorities and adds bypassable serialization seams.
3. **Literal everything-is-a-plugin** — makes the privacy policy replaceable by the code it must constrain.
4. **Global persistent token/alias vault** — duplicates sensitive source data and creates another high-value authority; use retained source plus scoped value-free bindings.
5. **Dynamic Go `.so` as v1 plugin architecture** — poor portability, version coupling and no isolation benefit; begin with static first-party registration and add isolated external execution only when required.

## 7. Recheck triggers

Re-run this review before changing the final architecture if an upstream ships a stable incompatible extension or privacy model, if Analytix introduces third-party executable plugins, or if formal performance evidence shows the single-Harness projection model cannot meet required latency. A newer upstream commit is evidence to recheck, not automatic authority to migrate.

## 8. Revalidation addendum — 2026-08-26

The same four-workstream Exa research set was re-evaluated against official
documentation and repositories. A fresh `git ls-remote <official-repo> HEAD`
read during this review observed:

| Upstream | 2026-08-26 observed HEAD | Delta from Section 1 |
| --- | --- | --- |
| OpenAI Codex | [`20c3f9733f7d7ee51e0eb26d82265db78c648335`](https://github.com/openai/codex/tree/20c3f9733f7d7ee51e0eb26d82265db78c648335) | Advanced; architecture conclusion unchanged. |
| Anthropic Claude Code | [`1af51fd77b7e40017c1032249db75fe582d23283`](https://github.com/anthropics/claude-code/tree/1af51fd77b7e40017c1032249db75fe582d23283) | Unchanged. |
| DeepSeek Harness | [`b150a551b8d465e31e418e1b2eaf5e79bbb7d28e`](https://github.com/deepseek-ai/deepseek-harness/tree/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e) | Unchanged. |
| OpenCode | [`8615731d46153dd29b89e205fb55b2cc16205cb0`](https://github.com/anomalyco/opencode/tree/8615731d46153dd29b89e205fb55b2cc16205cb0) | Unchanged. |

These are moving tracking refs, not vendored or admitted source pins. Any code
reuse still requires the separate provenance and exact-artifact gate.

The material architecture delta is in Codex's now-explicit package-level
[plugin model](https://developers.openai.com/plugins/build/plugins): a plugin
can combine a manifest, skills, MCP/app declarations, hooks and assets, while
the [app-server](https://developers.openai.com/codex/app-server) remains the
Harness protocol for threads, turns, items, events and approvals. This
strengthens the extension-package model, but it does not make package
installation a sensitive-data authority. Experimental or evolving plugin APIs
also cannot be the Analytix privacy boundary.

The other three rechecks reinforce the existing rejection set:

- Claude Code's official [agent loop](https://code.claude.com/docs/en/agent-sdk/agent-loop),
  [permissions](https://code.claude.com/docs/en/permissions),
  [sandboxing](https://code.claude.com/docs/en/sandboxing) and
  [data usage](https://code.claude.com/docs/en/data-usage) keep execution,
  approval and sandboxing distinct, while local transcripts and sandbox
  availability still require stricter fail-closed treatment for sensitive work.
- DeepSeek Harness still provides the clearest capability/profile/bundle model,
  but its [session telemetry](https://github.com/deepseek-ai/deepseek-harness/blob/b150a551b8d465e31e418e1b2eaf5e79bbb7d28e/docs/subsystems/session-telemetry.md)
  confirms that complete event payloads require a separately installed
  redaction policy. Analytix cannot make that policy optional.
- OpenCode's official [plugins](https://opencode.ai/docs/plugins/) and
  [permissions](https://opencode.ai/docs/permissions/) remain useful for
  extension lifecycle and UX, while its pinned model-message lowering and
  compaction paths continue to show that raw durable history plus in-process
  plugin access is not a privacy architecture.

### 8.1 Refined ruling

The 2026-08-26 evidence does not justify a second runtime or a literal
everything-is-a-plugin Core. It adds one required distinction:

```text
plugin package plane
  manifest + skills + MCP/hooks/assets + public-safe UI
                         !=
privileged capability plane
  Go Core-issued typed grant + final authorization/projection/revocation
```

Installing a package never grants raw DuckDB/source data, raw history, direct
Provider/network/persistence/logging, or protected-local display. If required
process or OS isolation is unavailable, Analytix disables that executable
plugin lane instead of falling back to unsandboxed execution. The final ruling
therefore remains:

> **One Go Agent Harness + unbypassable Privacy Layer + capability-based Plugins.**
