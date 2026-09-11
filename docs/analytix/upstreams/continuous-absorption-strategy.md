# analytix continuous upstream absorption strategy

Status: Operational upstream-intake strategy.
Applies to: all sources registered in `upstream-sources.json` and every direct,
substantial, clean-room, vendored, generated, extracted, binary, or asset reuse
decision.
Current as of: 2026-07-10.
Source of truth: current Analytix code/tests, the pinned manifest, source-specific
ledgers, `code-reuse-provenance.md`, and fresh audit/benchmark evidence.

## North Star

analytix remains a private, independent desktop product on local branch `main`.
The sources registered in `upstream-sources.json` are research inputs. The
current intake contains eleven repositories; the manifest, not a prose count,
is the authoritative inventory.
They are not analytix remotes and they do not define the public product
surface.

The source labels below are default research lenses, not absorption limits. No
upstream owns a capability category exclusively. For every analytix capability,
all relevant upstreams may be compared, and the best proven design may be
absorbed through analytix-owned contracts.

The first hard performance goal is DeepSeek quality:

```text
analytix DeepSeek behavior must match DeepSeek-Reasonix first,
then beat it on reply speed, cache hit rate, and desktop integration.
```

No superiority claim is valid without benchmark evidence.

## Local Source Layout

```text
/Users/sun/Projects/analytix                  # private product repo, branch main
/Users/sun/Projects/_upstreams/Kun
/Users/sun/Projects/_upstreams/DeepSeek-Reasonix
/Users/sun/Projects/_upstreams/opencode
/Users/sun/Projects/_upstreams/CodexDesktop-Rebuild
/Users/sun/Projects/_upstreams/hermes-agent
/Users/sun/Projects/_upstreams/claude-code
/Users/sun/Projects/_upstreams/claw-code
/Users/sun/Projects/_upstreams/gajae-code
/Users/sun/Projects/_upstreams/lazycodex
```

## Default Research Lenses

These lenses help choose where to look first. They must not be used to block
another upstream from informing the same capability when evidence says it is
better.

| Source | Default lens | What analytix may learn | What must not leak |
| --- | --- | --- | --- |
| Kun | Product workflow and inherited desktop behavior | Code, Write, SDD, Connect Phone, schedule, GUI command behavior, provider UX, desktop workflow bug fixes. | Kun identity, old bridge/settings schema, unapproved upstream product surface, old runtime-control panels. |
| DeepSeek-Reasonix | Runtime, DeepSeek, and engine discipline | DeepSeek request/stream/usage/cache discipline, Go runtime ideas, MCP/tool lifecycle, approvals, long tasks, context economy. | Reasonix public protocol, CLI identity, settings roots, terminal-only product assumptions. |
| OpenCode | Agent architecture comparison | Provider registry breadth, sub-agent permission derivation, task/TODO/session continuation, background task metadata. | OpenCode product protocol, weaker permissions, UI shell, unreviewed task semantics. |
| CodexDesktop-Rebuild | Desktop feel and responsiveness | Thread virtualizer, scroll controller, workers, app-server boundary, signal/query state, composer latency hiding, interaction rhythm. | Codex identity, minified chunk ownership, deletion of analytix workflows. |
| Hermes Agent | Learning loop and cross-channel operations | Skill evolution, memory/search, gateway delivery, scheduled automations, terminal backends, tool RPC, trajectory compression. | Hermes CLI/config/memory protocol, messaging identity, product surface. |
| Claude Code | Behavior and enterprise UX baseline | Durable attach/resume, background-agent UX, managed settings, remote-control and plugin-security requirements. | Any copied code, prompt, plugin/docs text, binary, asset, protocol, or identity absent written authorization. |
| Claw Code | Low-confidence Rust/fixture comparison | Small parser, lifecycle, status, cancellation, and workspace test ideas after code verification. | In-memory registry behavior presented as durable parity, shell heuristics as a safety boundary, Claw identity. |
| Gajae Code | Orchestration, plugin quarantine, external control, and native performance | Session tree/handoff, coordinator/RPC/ACP contracts, plugin compile/validate/quarantine, receipts, notifications, scan/AST/PTY benchmark ideas. | GJC protocol/product identity, Bun runtime insertion, or lineage code before copyright provenance is resolved. |
| LazyCodex | Codex plugin component and execution-discipline reference | LSP/CodeGraph plugin boundaries, team/worktree lifecycle, bootstrap/degraded state, continuation, evidence verification. | Unreviewed OmO gitlink code, opt-out telemetry, broad default prompt injection, third-party component code without its own notice. |

## License And Provenance Gate

Before direct reuse, the batch must record the source repository and full
commit, exact source/destination paths, source blob hashes, reuse mode,
file-level license and copyright holders, NOTICE/attribution destination,
modifications, reviewer, and validation in `code-reuse-provenance.md`.

No project license or an all-rights-reserved notice means clean-room behavior
study only unless separate written authorization covers the exact material.
MIT/Apache/BSD roots remain subject to gitlink, vendored, generated, extracted,
and file-level exceptions. A hash refresh without a behavior and license delta
review does not reopen a blocked source.

## Capability-First Rule

Every upstream study starts from an analytix capability, not from a source
territory. The comparison set is open by default:

```text
analytix capability
  -> compare current analytix behavior
  -> compare all relevant upstream implementations or ideas
  -> choose the strongest proven design
  -> adapt it into analytix-owned contracts, UI, runtime, tests, and QA
```

If a source produces the best idea outside its default lens, analytix may still
absorb it. If a default lens source is weaker for a capability, analytix should
record the comparison and choose another design.

## Continuous Loop

```text
capability selection
  -> upstream refresh
  -> all-relevant-source comparison
  -> capability classification
  -> conflict decision
  -> analytix contract landing
  -> implementation branch
  -> focused tests and benchmarks
  -> desktop QA when UI/streaming changes
  -> absorption ledger and release scorecard
  -> merge to main
```

Every batch must answer:

```text
What changed upstream?
Which analytix capability does it improve?
Which upstreams were compared, and where do they disagree?
What does analytix keep, adapt, reject, or defer?
Which contract owns the landing?
Which test, benchmark, or QA artifact proves the result?
```

## DeepSeek Speed And Cache Program

This is the highest-priority engine program.

### Required Metrics

| Metric | Why it matters |
| --- | --- |
| Time to first token | User-visible responsiveness. |
| Tokens per second | Perceived model speed and long-answer throughput. |
| End-to-end turn latency | Real desktop experience, including prompt build, tool setup, stream parsing, projection, and rendering. |
| Cache hit tokens and miss tokens | Direct DeepSeek cache efficiency signal. |
| Cache hit rate by stable prefix hash | Proves stable system/tool/context prefix discipline. |
| Prefix-change reason | Prevents accidental cache busting by dynamic workspace data, timestamps, selected text, or tool schema churn. |
| Provider/model/endpoint attribution | Avoids mixing DeepSeek evidence with OpenAI-compatible or unsupported providers. |
| Renderer streaming frame cost | Prevents backend speed gains from being hidden by UI rendering jank. |

### Required Gates

```text
1. Snapshot DeepSeek-Reasonix current request/cache behavior.
2. Snapshot analytix DeepSeek request/cache behavior.
3. Compare URL, body, headers, stream frames, reasoning fields, usage fields.
4. Freeze stable prefix and canonical tool schema hashes.
5. Run warm/cold cache benchmark across repeated equivalent turns.
6. Run long-thread benchmark with compaction and tool availability changes.
7. Run desktop streaming QA to prove speed is visible in the UI.
8. Record scorecard before claiming parity or superiority.
```

### Implementation Bias

- Keep dynamic workspace data out of the stable prefix.
- Keep tool schemas canonical and sorted.
- Keep provider/model/endpoint changes explicitly attributed.
- Parse unsupported cache telemetry as unknown, not as zero.
- Separate backend model latency from renderer projection/render latency.
- Prefer improving prompt/cache discipline before adding more model calls.

## Product Workflow Comparison Program

Kun is a mandatory comparison input for inherited product workflows, not the
exclusive UI authority and not the Git base. Each Kun update must be checked
for:

```text
Code / Write / SDD / Connect Phone / Schedule
composer commands
model picker and reasoning controls
provider presets and probes
file edit review / generated files / checkpoints
tray, branch, worktree, and session utilities
```

Analytix may absorb product UI or workflow ideas from any source after
identifying the exact source behavior, analytix landing surface, proof that it
fits the upgraded UI, and tests that preserve existing interactions. Kun
remains the regression floor for inherited workflows; it is not a ceiling for
future analytix product capabilities.

## Agent Architecture Comparison Program

For sub-agent, TODO/task, background job, and long-running work, compare all
relevant sources before implementation:

```text
DeepSeek-Reasonix -> engine discipline and Go/runtime ideas
OpenCode          -> provider/task/session continuation design
Hermes Agent      -> learning loop, tool RPC, subagent and gateway behavior
Gajae Code        -> durable receipts, orchestration, plugin quarantine, external control
LazyCodex         -> LSP/CodeGraph plugin and evidence-verification components
Claude Code       -> clean-room behavior baseline for daemon/background/enterprise UX
Claw Code         -> low-confidence fixture and failure-mode comparison
Kun               -> product entry, workflow, and user expectation checks
CodexDesktop      -> nested task UI, responsiveness, and interaction cost
analytix current  -> desktop product contract and stricter safety rules
```

Analytix should implement the smallest contract that wins on:

```text
permission safety
durable restart
parent/child thread lineage
TODO/goal evidence
model/provider inheritance
cache stability
desktop renderability
user-controllable interruption
```

## Desktop Feel Program

CodexDesktop-Rebuild is the default starting reference for feel, not the only
source of desktop architecture or interaction ideas. Absorption must be
measured through real desktop or browser QA:

```text
long thread scroll stability
streaming without forced-scroll
composer typing latency
sidebar search/list responsiveness
markdown/code/diff render cost
worker offload effectiveness
panel open/close and resize behavior
```

Renderer changes must preserve analytix workflows and route surfaces.

## Release Claim Rules

Do not claim a release is better than an upstream unless the relevant ledger,
tests, and scorecard prove it:

| Claim | Evidence required |
| --- | --- |
| Better DeepSeek than Reasonix | Speed/cache benchmark, request/stream/usage parity, stable prefix proof, desktop streaming QA. |
| Better product workflow than Kun | Kun sync ledger, workflow benchmark, renderer tests, desktop QA, no identity/schema regression. |
| Better sub-agent/TODO system than OpenCode/Reasonix/Hermes | Three-way comparison, permission fixtures, durable lineage/restart tests, renderer projection QA. |
| Better desktop feel than CodexDesktop-Rebuild reference | Measured long-thread/streaming/composer responsiveness with screenshots or trace evidence. |
| Better self-improving agent behavior than Hermes ideas | analytix-owned memory/skill contracts, privacy review, opt-in controls, benchmarked task reuse. |
| Better persistence than Reasonix/OpenCode | Cross-process lease, atomic replace, revision/CAS, conflict recovery, event-index rebuild, retention, restart and Windows fault-injection proof. |
| Better code intelligence than OpenCode/Lazy/Gajae | LSP/formatter/CodeGraph correctness, process cleanup, workspace trust, schema/token cost, and task-success benchmark. |
| Better external control than Gajae/Claude behavior | Analytix-owned capability negotiation, authentication, permissions, unattended policy, recovery, redaction, and desktop coexistence tests. |

## Branch And Git Policy

- `main` is the analytix mainline.
- Upstream projects stay outside analytix as `/Users/sun/Projects/_upstreams/*`.
- Upstream sources must not be configured as analytix Git remotes.
- Absorption work uses focused branches from `main`.
- Merge by analytix contract proof, not by upstream diff size.
